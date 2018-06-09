// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/auth"
	"code.gitea.io/gitea/modules/auth/oauth2"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"

	"github.com/go-macaron/captcha"
	"github.com/markbates/goth"
	"github.com/tstranex/u2f"
)

const (
	// tplSignIn template for sign in page
	tplSignIn base.TplName = "user/auth/signin"
	// tplSignUp template path for sign up page
	tplSignUp base.TplName = "user/auth/signup"
	// TplActivate template path for activate user
	TplActivate       base.TplName = "user/auth/activate"
	tplForgotPassword base.TplName = "user/auth/forgot_passwd"
	tplResetPassword  base.TplName = "user/auth/reset_passwd"
	tplTwofa          base.TplName = "user/auth/twofa"
	tplTwofaScratch   base.TplName = "user/auth/twofa_scratch"
	tplLinkAccount    base.TplName = "user/auth/link_account"
	tplU2F            base.TplName = "user/auth/u2f"
)

// AutoSignIn reads cookie and try to auto-login.
func AutoSignIn(ctx *context.Context) (bool, error) {
	if !models.HasEngine {
		return false, nil
	}

	uname := ctx.GetCookie(setting.CookieUserName)
	if len(uname) == 0 {
		return false, nil
	}

	isSucceed := false
	defer func() {
		if !isSucceed {
			log.Trace("auto-login cookie cleared: %s", uname)
			ctx.SetCookie(setting.CookieUserName, "", -1, setting.AppSubURL)
			ctx.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubURL)
		}
	}()

	u, err := models.GetUserByName(uname)
	if err != nil {
		if !models.IsErrUserNotExist(err) {
			return false, fmt.Errorf("GetUserByName: %v", err)
		}
		return false, nil
	}

	if val, _ := ctx.GetSuperSecureCookie(
		base.EncodeMD5(u.Rands+u.Passwd), setting.CookieRememberName); val != u.Name {
		return false, nil
	}

	isSucceed = true
	ctx.Session.Set("uid", u.ID)
	ctx.Session.Set("uname", u.Name)
	ctx.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	return true, nil
}

func checkAutoLogin(ctx *context.Context) bool {
	// Check auto-login.
	isSucceed, err := AutoSignIn(ctx)
	if err != nil {
		ctx.ServerError("AutoSignIn", err)
		return true
	}

	redirectTo := ctx.Query("redirect_to")
	if len(redirectTo) > 0 {
		ctx.SetCookie("redirect_to", redirectTo, 0, setting.AppSubURL)
	} else {
		redirectTo, _ = url.QueryUnescape(ctx.GetCookie("redirect_to"))
	}

	if isSucceed {
		ctx.SetCookie("redirect_to", "", -1, setting.AppSubURL)
		ctx.RedirectToFirst(redirectTo, setting.AppSubURL+string(setting.LandingPageURL))
		return true
	}

	return false
}

// SignIn render sign in page
func SignIn(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("sign_in")

	// Check auto-login.
	if checkAutoLogin(ctx) {
		return
	}

	orderedOAuth2Names, oauth2Providers, err := models.GetActiveOAuth2Providers()
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}
	ctx.Data["OrderedOAuth2Names"] = orderedOAuth2Names
	ctx.Data["OAuth2Providers"] = oauth2Providers
	ctx.Data["Title"] = ctx.Tr("sign_in")
	ctx.Data["SignInLink"] = setting.AppSubURL + "/user/login"
	ctx.Data["PageIsSignIn"] = true
	ctx.Data["PageIsLogin"] = true

	ctx.HTML(200, tplSignIn)
}

// SignInPost response for sign in request
func SignInPost(ctx *context.Context, form auth.SignInForm) {
	ctx.Data["Title"] = ctx.Tr("sign_in")

	orderedOAuth2Names, oauth2Providers, err := models.GetActiveOAuth2Providers()
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}
	ctx.Data["OrderedOAuth2Names"] = orderedOAuth2Names
	ctx.Data["OAuth2Providers"] = oauth2Providers
	ctx.Data["Title"] = ctx.Tr("sign_in")
	ctx.Data["SignInLink"] = setting.AppSubURL + "/user/login"
	ctx.Data["PageIsSignIn"] = true
	ctx.Data["PageIsLogin"] = true

	if ctx.HasError() {
		ctx.HTML(200, tplSignIn)
		return
	}

	u, err := models.UserSignIn(form.UserName, form.Password)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.RenderWithErr(ctx.Tr("form.username_password_incorrect"), tplSignIn, &form)
			log.Info("Failed authentication attempt for %s from %s", form.UserName, ctx.RemoteAddr())
		} else if models.IsErrEmailAlreadyUsed(err) {
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), tplSignIn, &form)
			log.Info("Failed authentication attempt for %s from %s", form.UserName, ctx.RemoteAddr())
		} else {
			ctx.ServerError("UserSignIn", err)
		}
		return
	}
	// If this user is enrolled in 2FA, we can't sign the user in just yet.
	// Instead, redirect them to the 2FA authentication page.
	_, err = models.GetTwoFactorByUID(u.ID)
	if err != nil {
		if models.IsErrTwoFactorNotEnrolled(err) {
			handleSignIn(ctx, u, form.Remember)
		} else {
			ctx.ServerError("UserSignIn", err)
		}
		return
	}

	// User needs to use 2FA, save data and redirect to 2FA page.
	ctx.Session.Set("twofaUid", u.ID)
	ctx.Session.Set("twofaRemember", form.Remember)

	regs, err := models.GetU2FRegistrationsByUID(u.ID)
	if err == nil && len(regs) > 0 {
		ctx.Redirect(setting.AppSubURL + "/user/u2f")
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/two_factor")
}

// TwoFactor shows the user a two-factor authentication page.
func TwoFactor(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("twofa")

	// Check auto-login.
	if checkAutoLogin(ctx) {
		return
	}

	// Ensure user is in a 2FA session.
	if ctx.Session.Get("twofaUid") == nil {
		ctx.ServerError("UserSignIn", errors.New("not in 2FA session"))
		return
	}

	ctx.HTML(200, tplTwofa)
}

// TwoFactorPost validates a user's two-factor authentication token.
func TwoFactorPost(ctx *context.Context, form auth.TwoFactorAuthForm) {
	ctx.Data["Title"] = ctx.Tr("twofa")

	// Ensure user is in a 2FA session.
	idSess := ctx.Session.Get("twofaUid")
	if idSess == nil {
		ctx.ServerError("UserSignIn", errors.New("not in 2FA session"))
		return
	}

	id := idSess.(int64)
	twofa, err := models.GetTwoFactorByUID(id)
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}

	// Validate the passcode with the stored TOTP secret.
	ok, err := twofa.ValidateTOTP(form.Passcode)
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}

	if ok && twofa.LastUsedPasscode != form.Passcode {
		remember := ctx.Session.Get("twofaRemember").(bool)
		u, err := models.GetUserByID(id)
		if err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}

		if ctx.Session.Get("linkAccount") != nil {
			gothUser := ctx.Session.Get("linkAccountGothUser")
			if gothUser == nil {
				ctx.ServerError("UserSignIn", errors.New("not in LinkAccount session"))
				return
			}

			err = models.LinkAccountToUser(u, gothUser.(goth.User))
			if err != nil {
				ctx.ServerError("UserSignIn", err)
				return
			}
		}

		twofa.LastUsedPasscode = form.Passcode
		if err = models.UpdateTwoFactor(twofa); err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}

		handleSignIn(ctx, u, remember)
		return
	}

	ctx.RenderWithErr(ctx.Tr("auth.twofa_passcode_incorrect"), tplTwofa, auth.TwoFactorAuthForm{})
}

// TwoFactorScratch shows the scratch code form for two-factor authentication.
func TwoFactorScratch(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("twofa_scratch")

	// Check auto-login.
	if checkAutoLogin(ctx) {
		return
	}

	// Ensure user is in a 2FA session.
	if ctx.Session.Get("twofaUid") == nil {
		ctx.ServerError("UserSignIn", errors.New("not in 2FA session"))
		return
	}

	ctx.HTML(200, tplTwofaScratch)
}

// TwoFactorScratchPost validates and invalidates a user's two-factor scratch token.
func TwoFactorScratchPost(ctx *context.Context, form auth.TwoFactorScratchAuthForm) {
	ctx.Data["Title"] = ctx.Tr("twofa_scratch")

	// Ensure user is in a 2FA session.
	idSess := ctx.Session.Get("twofaUid")
	if idSess == nil {
		ctx.ServerError("UserSignIn", errors.New("not in 2FA session"))
		return
	}

	id := idSess.(int64)
	twofa, err := models.GetTwoFactorByUID(id)
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}

	// Validate the passcode with the stored TOTP secret.
	if twofa.VerifyScratchToken(form.Token) {
		// Invalidate the scratch token.
		twofa.ScratchToken = ""
		if err = models.UpdateTwoFactor(twofa); err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}

		remember := ctx.Session.Get("twofaRemember").(bool)
		u, err := models.GetUserByID(id)
		if err != nil {
			ctx.ServerError("UserSignIn", err)
			return
		}

		handleSignInFull(ctx, u, remember, false)
		ctx.Flash.Info(ctx.Tr("auth.twofa_scratch_used"))
		ctx.Redirect(setting.AppSubURL + "/user/settings/two_factor")
		return
	}

	ctx.RenderWithErr(ctx.Tr("auth.twofa_scratch_token_incorrect"), tplTwofaScratch, auth.TwoFactorScratchAuthForm{})
}

// U2F shows the U2F login page
func U2F(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("twofa")
	ctx.Data["RequireU2F"] = true
	// Check auto-login.
	if checkAutoLogin(ctx) {
		return
	}

	// Ensure user is in a 2FA session.
	if ctx.Session.Get("twofaUid") == nil {
		ctx.ServerError("UserSignIn", errors.New("not in U2F session"))
		return
	}

	ctx.HTML(200, tplU2F)
}

// U2FChallenge submits a sign challenge to the browser
func U2FChallenge(ctx *context.Context) {
	// Ensure user is in a U2F session.
	idSess := ctx.Session.Get("twofaUid")
	if idSess == nil {
		ctx.ServerError("UserSignIn", errors.New("not in U2F session"))
		return
	}
	id := idSess.(int64)
	regs, err := models.GetU2FRegistrationsByUID(id)
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}
	if len(regs) == 0 {
		ctx.ServerError("UserSignIn", errors.New("no device registered"))
		return
	}
	challenge, err := u2f.NewChallenge(setting.U2F.AppID, setting.U2F.TrustedFacets)
	if err = ctx.Session.Set("u2fChallenge", challenge); err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}
	ctx.JSON(200, challenge.SignRequest(regs.ToRegistrations()))
}

// U2FSign authenticates the user by signResp
func U2FSign(ctx *context.Context, signResp u2f.SignResponse) {
	challSess := ctx.Session.Get("u2fChallenge")
	idSess := ctx.Session.Get("twofaUid")
	if challSess == nil || idSess == nil {
		ctx.ServerError("UserSignIn", errors.New("not in U2F session"))
		return
	}
	challenge := challSess.(*u2f.Challenge)
	id := idSess.(int64)
	regs, err := models.GetU2FRegistrationsByUID(id)
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}
	for _, reg := range regs {
		r, err := reg.Parse()
		if err != nil {
			log.Fatal(4, "parsing u2f registration: %v", err)
			continue
		}
		newCounter, authErr := r.Authenticate(signResp, *challenge, reg.Counter)
		if authErr == nil {
			reg.Counter = newCounter
			user, err := models.GetUserByID(id)
			if err != nil {
				ctx.ServerError("UserSignIn", err)
				return
			}
			remember := ctx.Session.Get("twofaRemember").(bool)
			if err := reg.UpdateCounter(); err != nil {
				ctx.ServerError("UserSignIn", err)
				return
			}

			if ctx.Session.Get("linkAccount") != nil {
				gothUser := ctx.Session.Get("linkAccountGothUser")
				if gothUser == nil {
					ctx.ServerError("UserSignIn", errors.New("not in LinkAccount session"))
					return
				}

				err = models.LinkAccountToUser(user, gothUser.(goth.User))
				if err != nil {
					ctx.ServerError("UserSignIn", err)
					return
				}
			}
			redirect := handleSignInFull(ctx, user, remember, false)
			if redirect == "" {
				redirect = setting.AppSubURL + "/"
			}
			ctx.PlainText(200, []byte(redirect))
			return
		}
	}
	ctx.Error(401)
}

// This handles the final part of the sign-in process of the user.
func handleSignIn(ctx *context.Context, u *models.User, remember bool) {
	handleSignInFull(ctx, u, remember, true)
}

func handleSignInFull(ctx *context.Context, u *models.User, remember bool, obeyRedirect bool) string {
	if remember {
		days := 86400 * setting.LogInRememberDays
		ctx.SetCookie(setting.CookieUserName, u.Name, days, setting.AppSubURL)
		ctx.SetSuperSecureCookie(base.EncodeMD5(u.Rands+u.Passwd),
			setting.CookieRememberName, u.Name, days, setting.AppSubURL)
	}

	ctx.Session.Delete("openid_verified_uri")
	ctx.Session.Delete("openid_signin_remember")
	ctx.Session.Delete("openid_determined_email")
	ctx.Session.Delete("openid_determined_username")
	ctx.Session.Delete("twofaUid")
	ctx.Session.Delete("twofaRemember")
	ctx.Session.Delete("u2fChallenge")
	ctx.Session.Delete("linkAccount")
	ctx.Session.Set("uid", u.ID)
	ctx.Session.Set("uname", u.Name)

	// Language setting of the user overwrites the one previously set
	// If the user does not have a locale set, we save the current one.
	if len(u.Language) == 0 {
		u.Language = ctx.Locale.Language()
		if err := models.UpdateUserCols(u, "language"); err != nil {
			log.Error(4, fmt.Sprintf("Error updating user language [user: %d, locale: %s]", u.ID, u.Language))
			return setting.AppSubURL + "/"
		}
	}

	ctx.SetCookie("lang", u.Language, nil, setting.AppSubURL)

	// Clear whatever CSRF has right now, force to generate a new one
	ctx.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)

	// Register last login
	u.SetLastLogin()
	if err := models.UpdateUserCols(u, "last_login_unix"); err != nil {
		ctx.ServerError("UpdateUserCols", err)
		return setting.AppSubURL + "/"
	}

	if redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to")); len(redirectTo) > 0 {
		ctx.SetCookie("redirect_to", "", -1, setting.AppSubURL)
		if obeyRedirect {
			ctx.RedirectToFirst(redirectTo)
		}
		return redirectTo
	}

	if obeyRedirect {
		ctx.Redirect(setting.AppSubURL + "/")
	}
	return setting.AppSubURL + "/"
}

// SignInOAuth handles the OAuth2 login buttons
func SignInOAuth(ctx *context.Context) {
	provider := ctx.Params(":provider")

	loginSource, err := models.GetActiveOAuth2LoginSourceByName(provider)
	if err != nil {
		ctx.ServerError("SignIn", err)
		return
	}

	// try to do a direct callback flow, so we don't authenticate the user again but use the valid accesstoken to get the user
	user, gothUser, err := oAuth2UserLoginCallback(loginSource, ctx.Req.Request, ctx.Resp)
	if err == nil && user != nil {
		// we got the user without going through the whole OAuth2 authentication flow again
		handleOAuth2SignIn(user, gothUser, ctx, err)
		return
	}

	err = oauth2.Auth(loginSource.Name, ctx.Req.Request, ctx.Resp)
	if err != nil {
		ctx.ServerError("SignIn", err)
	}
	// redirect is done in oauth2.Auth
}

// SignInOAuthCallback handles the callback from the given provider
func SignInOAuthCallback(ctx *context.Context) {
	provider := ctx.Params(":provider")

	// first look if the provider is still active
	loginSource, err := models.GetActiveOAuth2LoginSourceByName(provider)
	if err != nil {
		ctx.ServerError("SignIn", err)
		return
	}

	if loginSource == nil {
		ctx.ServerError("SignIn", errors.New("No valid provider found, check configured callback url in provider"))
		return
	}

	u, gothUser, err := oAuth2UserLoginCallback(loginSource, ctx.Req.Request, ctx.Resp)

	handleOAuth2SignIn(u, gothUser, ctx, err)
}

func handleOAuth2SignIn(u *models.User, gothUser goth.User, ctx *context.Context, err error) {
	if err != nil {
		ctx.ServerError("UserSignIn", err)
		return
	}

	if u == nil {
		// no existing user is found, request attach or new account
		ctx.Session.Set("linkAccountGothUser", gothUser)
		ctx.Redirect(setting.AppSubURL + "/user/link_account")
		return
	}

	// If this user is enrolled in 2FA, we can't sign the user in just yet.
	// Instead, redirect them to the 2FA authentication page.
	_, err = models.GetTwoFactorByUID(u.ID)
	if err != nil {
		if models.IsErrTwoFactorNotEnrolled(err) {
			ctx.Session.Set("uid", u.ID)
			ctx.Session.Set("uname", u.Name)

			// Clear whatever CSRF has right now, force to generate a new one
			ctx.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)

			// Register last login
			u.SetLastLogin()
			if err := models.UpdateUserCols(u, "last_login_unix"); err != nil {
				ctx.ServerError("UpdateUserCols", err)
				return
			}

			if redirectTo, _ := url.QueryUnescape(ctx.GetCookie("redirect_to")); len(redirectTo) > 0 {
				ctx.SetCookie("redirect_to", "", -1, setting.AppSubURL)
				ctx.RedirectToFirst(redirectTo)
				return
			}

			ctx.Redirect(setting.AppSubURL + "/")
		} else {
			ctx.ServerError("UserSignIn", err)
		}
		return
	}

	// User needs to use 2FA, save data and redirect to 2FA page.
	ctx.Session.Set("twofaUid", u.ID)
	ctx.Session.Set("twofaRemember", false)

	// If U2F is enrolled -> Redirect to U2F instead
	regs, err := models.GetU2FRegistrationsByUID(u.ID)
	if err == nil && len(regs) > 0 {
		ctx.Redirect(setting.AppSubURL + "/user/u2f")
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/two_factor")
}

// OAuth2UserLoginCallback attempts to handle the callback from the OAuth2 provider and if successful
// login the user
func oAuth2UserLoginCallback(loginSource *models.LoginSource, request *http.Request, response http.ResponseWriter) (*models.User, goth.User, error) {
	gothUser, err := oauth2.ProviderCallback(loginSource.Name, request, response)

	if err != nil {
		return nil, goth.User{}, err
	}

	user := &models.User{
		LoginName:   gothUser.UserID,
		LoginType:   models.LoginOAuth2,
		LoginSource: loginSource.ID,
	}

	hasUser, err := models.GetUser(user)
	if err != nil {
		return nil, goth.User{}, err
	}

	if hasUser {
		return user, goth.User{}, nil
	}

	// search in external linked users
	externalLoginUser := &models.ExternalLoginUser{
		ExternalID:    gothUser.UserID,
		LoginSourceID: loginSource.ID,
	}
	hasUser, err = models.GetExternalLogin(externalLoginUser)
	if err != nil {
		return nil, goth.User{}, err
	}
	if hasUser {
		user, err = models.GetUserByID(externalLoginUser.UserID)
		return user, goth.User{}, err
	}

	// no user found to login
	return nil, gothUser, nil

}

// LinkAccount shows the page where the user can decide to login or create a new account
func LinkAccount(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("link_account")
	ctx.Data["LinkAccountMode"] = true
	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha
	ctx.Data["DisableRegistration"] = setting.Service.DisableRegistration
	ctx.Data["ShowRegistrationButton"] = false

	// use this to set the right link into the signIn and signUp templates in the link_account template
	ctx.Data["SignInLink"] = setting.AppSubURL + "/user/link_account_signin"
	ctx.Data["SignUpLink"] = setting.AppSubURL + "/user/link_account_signup"

	gothUser := ctx.Session.Get("linkAccountGothUser")
	if gothUser == nil {
		ctx.ServerError("UserSignIn", errors.New("not in LinkAccount session"))
		return
	}

	ctx.Data["user_name"] = gothUser.(goth.User).NickName
	ctx.Data["email"] = gothUser.(goth.User).Email

	ctx.HTML(200, tplLinkAccount)
}

// LinkAccountPostSignIn handle the coupling of external account with another account using signIn
func LinkAccountPostSignIn(ctx *context.Context, signInForm auth.SignInForm) {
	ctx.Data["Title"] = ctx.Tr("link_account")
	ctx.Data["LinkAccountMode"] = true
	ctx.Data["LinkAccountModeSignIn"] = true
	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha
	ctx.Data["DisableRegistration"] = setting.Service.DisableRegistration
	ctx.Data["ShowRegistrationButton"] = false

	// use this to set the right link into the signIn and signUp templates in the link_account template
	ctx.Data["SignInLink"] = setting.AppSubURL + "/user/link_account_signin"
	ctx.Data["SignUpLink"] = setting.AppSubURL + "/user/link_account_signup"

	gothUser := ctx.Session.Get("linkAccountGothUser")
	if gothUser == nil {
		ctx.ServerError("UserSignIn", errors.New("not in LinkAccount session"))
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, tplLinkAccount)
		return
	}

	u, err := models.UserSignIn(signInForm.UserName, signInForm.Password)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.RenderWithErr(ctx.Tr("form.username_password_incorrect"), tplLinkAccount, &signInForm)
		} else {
			ctx.ServerError("UserLinkAccount", err)
		}
		return
	}

	// If this user is enrolled in 2FA, we can't sign the user in just yet.
	// Instead, redirect them to the 2FA authentication page.
	_, err = models.GetTwoFactorByUID(u.ID)
	if err != nil {
		if models.IsErrTwoFactorNotEnrolled(err) {
			err = models.LinkAccountToUser(u, gothUser.(goth.User))
			if err != nil {
				ctx.ServerError("UserLinkAccount", err)
			} else {
				handleSignIn(ctx, u, signInForm.Remember)
			}
		} else {
			ctx.ServerError("UserLinkAccount", err)
		}
		return
	}

	// User needs to use 2FA, save data and redirect to 2FA page.
	ctx.Session.Set("twofaUid", u.ID)
	ctx.Session.Set("twofaRemember", signInForm.Remember)
	ctx.Session.Set("linkAccount", true)

	// If U2F is enrolled -> Redirect to U2F instead
	regs, err := models.GetU2FRegistrationsByUID(u.ID)
	if err == nil && len(regs) > 0 {
		ctx.Redirect(setting.AppSubURL + "/user/u2f")
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/two_factor")
}

// LinkAccountPostRegister handle the creation of a new account for an external account using signUp
func LinkAccountPostRegister(ctx *context.Context, cpt *captcha.Captcha, form auth.RegisterForm) {
	ctx.Data["Title"] = ctx.Tr("link_account")
	ctx.Data["LinkAccountMode"] = true
	ctx.Data["LinkAccountModeRegister"] = true
	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha
	ctx.Data["DisableRegistration"] = setting.Service.DisableRegistration
	ctx.Data["ShowRegistrationButton"] = false

	// use this to set the right link into the signIn and signUp templates in the link_account template
	ctx.Data["SignInLink"] = setting.AppSubURL + "/user/link_account_signin"
	ctx.Data["SignUpLink"] = setting.AppSubURL + "/user/link_account_signup"

	gothUser := ctx.Session.Get("linkAccountGothUser")
	if gothUser == nil {
		ctx.ServerError("UserSignUp", errors.New("not in LinkAccount session"))
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, tplLinkAccount)
		return
	}

	if setting.Service.DisableRegistration {
		ctx.Error(403)
		return
	}

	if setting.Service.EnableCaptcha && !cpt.VerifyReq(ctx.Req) {
		ctx.Data["Err_Captcha"] = true
		ctx.RenderWithErr(ctx.Tr("form.captcha_incorrect"), tplLinkAccount, &form)
		return
	}

	if (len(strings.TrimSpace(form.Password)) > 0 || len(strings.TrimSpace(form.Retype)) > 0) && form.Password != form.Retype {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), tplLinkAccount, &form)
		return
	}
	if len(strings.TrimSpace(form.Password)) > 0 && len(form.Password) < setting.MinPasswordLength {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("auth.password_too_short", setting.MinPasswordLength), tplLinkAccount, &form)
		return
	}

	loginSource, err := models.GetActiveOAuth2LoginSourceByName(gothUser.(goth.User).Provider)
	if err != nil {
		ctx.ServerError("CreateUser", err)
	}

	u := &models.User{
		Name:        form.UserName,
		Email:       form.Email,
		Passwd:      form.Password,
		IsActive:    !setting.Service.RegisterEmailConfirm,
		LoginType:   models.LoginOAuth2,
		LoginSource: loginSource.ID,
		LoginName:   gothUser.(goth.User).UserID,
	}

	if err := models.CreateUser(u); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), tplLinkAccount, &form)
		case models.IsErrEmailAlreadyUsed(err):
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), tplLinkAccount, &form)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_reserved", err.(models.ErrNameReserved).Name), tplLinkAccount, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), tplLinkAccount, &form)
		default:
			ctx.ServerError("CreateUser", err)
		}
		return
	}
	log.Trace("Account created: %s", u.Name)

	// Auto-set admin for the only user.
	if models.CountUsers() == 1 {
		u.IsAdmin = true
		u.IsActive = true
		u.SetLastLogin()
		if err := models.UpdateUserCols(u, "is_admin", "is_active", "last_login_unix"); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}
	}

	// Send confirmation email
	if setting.Service.RegisterEmailConfirm && u.ID > 1 {
		models.SendActivateAccountMail(ctx.Context, u)
		ctx.Data["IsSendRegisterMail"] = true
		ctx.Data["Email"] = u.Email
		ctx.Data["ActiveCodeLives"] = base.MinutesToFriendly(setting.Service.ActiveCodeLives, ctx.Locale.Language())
		ctx.HTML(200, TplActivate)

		if err := ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
			log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
		}
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/login")
}

// SignOut sign out from login status
func SignOut(ctx *context.Context) {
	ctx.Session.Delete("uid")
	ctx.Session.Delete("uname")
	ctx.Session.Delete("socialId")
	ctx.Session.Delete("socialName")
	ctx.Session.Delete("socialEmail")
	ctx.SetCookie(setting.CookieUserName, "", -1, setting.AppSubURL)
	ctx.SetCookie(setting.CookieRememberName, "", -1, setting.AppSubURL)
	ctx.SetCookie(setting.CSRFCookieName, "", -1, setting.AppSubURL)
	ctx.SetCookie("lang", "", -1, setting.AppSubURL) // Setting the lang cookie will trigger the middleware to reset the language ot previous state.
	ctx.Redirect(setting.AppSubURL + "/")
}

// SignUp render the register page
func SignUp(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("sign_up")

	ctx.Data["SignUpLink"] = setting.AppSubURL + "/user/sign_up"

	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	ctx.Data["DisableRegistration"] = setting.Service.DisableRegistration

	ctx.HTML(200, tplSignUp)
}

// SignUpPost response for sign up information submission
func SignUpPost(ctx *context.Context, cpt *captcha.Captcha, form auth.RegisterForm) {
	ctx.Data["Title"] = ctx.Tr("sign_up")

	ctx.Data["SignUpLink"] = setting.AppSubURL + "/user/sign_up"

	ctx.Data["EnableCaptcha"] = setting.Service.EnableCaptcha

	//Permission denied if DisableRegistration or AllowOnlyExternalRegistration options are true
	if !setting.Service.ShowRegistrationButton {
		ctx.Error(403)
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, tplSignUp)
		return
	}

	if setting.Service.EnableCaptcha && !cpt.VerifyReq(ctx.Req) {
		ctx.Data["Err_Captcha"] = true
		ctx.RenderWithErr(ctx.Tr("form.captcha_incorrect"), tplSignUp, &form)
		return
	}

	if form.Password != form.Retype {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("form.password_not_match"), tplSignUp, &form)
		return
	}
	if len(form.Password) < setting.MinPasswordLength {
		ctx.Data["Err_Password"] = true
		ctx.RenderWithErr(ctx.Tr("auth.password_too_short", setting.MinPasswordLength), tplSignUp, &form)
		return
	}

	u := &models.User{
		Name:     form.UserName,
		Email:    form.Email,
		Passwd:   form.Password,
		IsActive: !setting.Service.RegisterEmailConfirm,
	}
	if err := models.CreateUser(u); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), tplSignUp, &form)
		case models.IsErrEmailAlreadyUsed(err):
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), tplSignUp, &form)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_reserved", err.(models.ErrNameReserved).Name), tplSignUp, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("user.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), tplSignUp, &form)
		default:
			ctx.ServerError("CreateUser", err)
		}
		return
	}
	log.Trace("Account created: %s", u.Name)

	// Auto-set admin for the only user.
	if models.CountUsers() == 1 {
		u.IsAdmin = true
		u.IsActive = true
		u.SetLastLogin()
		if err := models.UpdateUserCols(u, "is_admin", "is_active", "last_login_unix"); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}
	}

	// Send confirmation email, no need for social account.
	if setting.Service.RegisterEmailConfirm && u.ID > 1 {
		models.SendActivateAccountMail(ctx.Context, u)
		ctx.Data["IsSendRegisterMail"] = true
		ctx.Data["Email"] = u.Email
		ctx.Data["ActiveCodeLives"] = base.MinutesToFriendly(setting.Service.ActiveCodeLives, ctx.Locale.Language())
		ctx.HTML(200, TplActivate)

		if err := ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
			log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
		}
		return
	}

	ctx.Redirect(setting.AppSubURL + "/user/login")
}

// Activate render activate user page
func Activate(ctx *context.Context) {
	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Data["IsActivatePage"] = true
		if ctx.User.IsActive {
			ctx.Error(404)
			return
		}
		// Resend confirmation email.
		if setting.Service.RegisterEmailConfirm {
			if ctx.Cache.IsExist("MailResendLimit_" + ctx.User.LowerName) {
				ctx.Data["ResendLimited"] = true
			} else {
				ctx.Data["ActiveCodeLives"] = base.MinutesToFriendly(setting.Service.ActiveCodeLives, ctx.Locale.Language())
				models.SendActivateAccountMail(ctx.Context, ctx.User)

				if err := ctx.Cache.Put("MailResendLimit_"+ctx.User.LowerName, ctx.User.LowerName, 180); err != nil {
					log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
				}
			}
		} else {
			ctx.Data["ServiceNotEnabled"] = true
		}
		ctx.HTML(200, TplActivate)
		return
	}

	// Verify code.
	if user := models.VerifyUserActiveCode(code); user != nil {
		user.IsActive = true
		var err error
		if user.Rands, err = models.GetUserSalt(); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}
		if err := models.UpdateUserCols(user, "is_active", "rands"); err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.Error(404)
			} else {
				ctx.ServerError("UpdateUser", err)
			}
			return
		}

		log.Trace("User activated: %s", user.Name)

		ctx.Session.Set("uid", user.ID)
		ctx.Session.Set("uname", user.Name)
		ctx.Redirect(setting.AppSubURL + "/")
		return
	}

	ctx.Data["IsActivateFailed"] = true
	ctx.HTML(200, TplActivate)
}

// ActivateEmail render the activate email page
func ActivateEmail(ctx *context.Context) {
	code := ctx.Query("code")
	emailStr := ctx.Query("email")

	// Verify code.
	if email := models.VerifyActiveEmailCode(code, emailStr); email != nil {
		if err := email.Activate(); err != nil {
			ctx.ServerError("ActivateEmail", err)
		}

		log.Trace("Email activated: %s", email.Email)
		ctx.Flash.Success(ctx.Tr("settings.add_email_success"))
	}

	ctx.Redirect(setting.AppSubURL + "/user/settings/email")
	return
}

// ForgotPasswd render the forget pasword page
func ForgotPasswd(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.forgot_password_title")

	if setting.MailService == nil {
		ctx.Data["IsResetDisable"] = true
		ctx.HTML(200, tplForgotPassword)
		return
	}

	email := ctx.Query("email")
	ctx.Data["Email"] = email

	ctx.Data["IsResetRequest"] = true
	ctx.HTML(200, tplForgotPassword)
}

// ForgotPasswdPost response for forget password request
func ForgotPasswdPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.forgot_password_title")

	if setting.MailService == nil {
		ctx.NotFound("ForgotPasswdPost", nil)
		return
	}
	ctx.Data["IsResetRequest"] = true

	email := ctx.Query("email")
	ctx.Data["Email"] = email

	u, err := models.GetUserByEmail(email)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Data["ResetPwdCodeLives"] = base.MinutesToFriendly(setting.Service.ResetPwdCodeLives, ctx.Locale.Language())
			ctx.Data["IsResetSent"] = true
			ctx.HTML(200, tplForgotPassword)
			return
		}

		ctx.ServerError("user.ResetPasswd(check existence)", err)
		return
	}

	if !u.IsLocal() && !u.IsOAuth2() {
		ctx.Data["Err_Email"] = true
		ctx.RenderWithErr(ctx.Tr("auth.non_local_account"), tplForgotPassword, nil)
		return
	}

	if ctx.Cache.IsExist("MailResendLimit_" + u.LowerName) {
		ctx.Data["ResendLimited"] = true
		ctx.HTML(200, tplForgotPassword)
		return
	}

	models.SendResetPasswordMail(ctx.Context, u)
	if err = ctx.Cache.Put("MailResendLimit_"+u.LowerName, u.LowerName, 180); err != nil {
		log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
	}

	ctx.Data["ResetPwdCodeLives"] = base.MinutesToFriendly(setting.Service.ResetPwdCodeLives, ctx.Locale.Language())
	ctx.Data["IsResetSent"] = true
	ctx.HTML(200, tplForgotPassword)
}

// ResetPasswd render the reset password page
func ResetPasswd(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.reset_password")

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code
	ctx.Data["IsResetForm"] = true
	ctx.HTML(200, tplResetPassword)
}

// ResetPasswdPost response from reset password request
func ResetPasswdPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("auth.reset_password")

	code := ctx.Query("code")
	if len(code) == 0 {
		ctx.Error(404)
		return
	}
	ctx.Data["Code"] = code

	if u := models.VerifyUserActiveCode(code); u != nil {
		// Validate password length.
		passwd := ctx.Query("password")
		if len(passwd) < setting.MinPasswordLength {
			ctx.Data["IsResetForm"] = true
			ctx.Data["Err_Password"] = true
			ctx.RenderWithErr(ctx.Tr("auth.password_too_short", setting.MinPasswordLength), tplResetPassword, nil)
			return
		}

		var err error
		if u.Rands, err = models.GetUserSalt(); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}
		if u.Salt, err = models.GetUserSalt(); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}
		u.HashPassword(passwd)
		if err := models.UpdateUserCols(u, "passwd", "rands", "salt"); err != nil {
			ctx.ServerError("UpdateUser", err)
			return
		}

		log.Trace("User password reset: %s", u.Name)
		ctx.Redirect(setting.AppSubURL + "/user/login")
		return
	}

	ctx.Data["IsResetFailed"] = true
	ctx.HTML(200, tplResetPassword)
}