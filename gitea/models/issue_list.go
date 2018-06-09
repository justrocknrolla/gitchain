// Copyright 2017 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import "fmt"

// IssueList defines a list of issues
type IssueList []*Issue

func (issues IssueList) getRepoIDs() []int64 {
	repoIDs := make(map[int64]struct{}, len(issues))
	for _, issue := range issues {
		if _, ok := repoIDs[issue.RepoID]; !ok {
			repoIDs[issue.RepoID] = struct{}{}
		}
	}
	return keysInt64(repoIDs)
}

func (issues IssueList) loadRepositories(e Engine) ([]*Repository, error) {
	if len(issues) == 0 {
		return nil, nil
	}

	repoIDs := issues.getRepoIDs()
	repoMaps := make(map[int64]*Repository, len(repoIDs))
	err := e.
		In("id", repoIDs).
		Find(&repoMaps)
	if err != nil {
		return nil, fmt.Errorf("find repository: %v", err)
	}

	for _, issue := range issues {
		issue.Repo = repoMaps[issue.RepoID]
	}
	return valuesRepository(repoMaps), nil
}

// LoadRepositories loads issues' all repositories
func (issues IssueList) LoadRepositories() ([]*Repository, error) {
	return issues.loadRepositories(x)
}

func (issues IssueList) getPosterIDs() []int64 {
	posterIDs := make(map[int64]struct{}, len(issues))
	for _, issue := range issues {
		if _, ok := posterIDs[issue.PosterID]; !ok {
			posterIDs[issue.PosterID] = struct{}{}
		}
	}
	return keysInt64(posterIDs)
}

func (issues IssueList) loadPosters(e Engine) error {
	if len(issues) == 0 {
		return nil
	}

	posterIDs := issues.getPosterIDs()
	posterMaps := make(map[int64]*User, len(posterIDs))
	err := e.
		In("id", posterIDs).
		Find(&posterMaps)
	if err != nil {
		return err
	}

	for _, issue := range issues {
		if issue.PosterID <= 0 {
			continue
		}
		var ok bool
		if issue.Poster, ok = posterMaps[issue.PosterID]; !ok {
			issue.Poster = NewGhostUser()
		}
	}
	return nil
}

func (issues IssueList) getIssueIDs() []int64 {
	var ids = make([]int64, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

func (issues IssueList) loadLabels(e Engine) error {
	if len(issues) == 0 {
		return nil
	}

	type LabelIssue struct {
		Label      *Label      `xorm:"extends"`
		IssueLabel *IssueLabel `xorm:"extends"`
	}

	var issueLabels = make(map[int64][]*Label, len(issues)*3)
	rows, err := e.Table("label").
		Join("LEFT", "issue_label", "issue_label.label_id = label.id").
		In("issue_label.issue_id", issues.getIssueIDs()).
		Asc("label.name").
		Rows(new(LabelIssue))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var labelIssue LabelIssue
		err = rows.Scan(&labelIssue)
		if err != nil {
			return err
		}
		issueLabels[labelIssue.IssueLabel.IssueID] = append(issueLabels[labelIssue.IssueLabel.IssueID], labelIssue.Label)
	}

	for _, issue := range issues {
		issue.Labels = issueLabels[issue.ID]
	}
	return nil
}

func (issues IssueList) getMilestoneIDs() []int64 {
	var ids = make(map[int64]struct{}, len(issues))
	for _, issue := range issues {
		if _, ok := ids[issue.MilestoneID]; !ok {
			ids[issue.MilestoneID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (issues IssueList) loadMilestones(e Engine) error {
	milestoneIDs := issues.getMilestoneIDs()
	if len(milestoneIDs) == 0 {
		return nil
	}

	milestoneMaps := make(map[int64]*Milestone, len(milestoneIDs))
	err := e.
		In("id", milestoneIDs).
		Find(&milestoneMaps)
	if err != nil {
		return err
	}

	for _, issue := range issues {
		issue.Milestone = milestoneMaps[issue.MilestoneID]
	}
	return nil
}

func (issues IssueList) loadAssignees(e Engine) error {
	if len(issues) == 0 {
		return nil
	}

	type AssigneeIssue struct {
		IssueAssignee *IssueAssignees `xorm:"extends"`
		Assignee      *User           `xorm:"extends"`
	}

	var assignees = make(map[int64][]*User, len(issues))
	rows, err := e.Table("issue_assignees").
		Join("INNER", "user", "`user`.id = `issue_assignees`.assignee_id").
		In("`issue_assignees`.issue_id", issues.getIssueIDs()).
		Rows(new(AssigneeIssue))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var assigneeIssue AssigneeIssue
		err = rows.Scan(&assigneeIssue)
		if err != nil {
			return err
		}

		assignees[assigneeIssue.IssueAssignee.IssueID] = append(assignees[assigneeIssue.IssueAssignee.IssueID], assigneeIssue.Assignee)
	}

	for _, issue := range issues {
		issue.Assignees = assignees[issue.ID]
	}
	return nil
}

func (issues IssueList) getPullIssueIDs() []int64 {
	var ids = make([]int64, 0, len(issues))
	for _, issue := range issues {
		if issue.IsPull && issue.PullRequest == nil {
			ids = append(ids, issue.ID)
		}
	}
	return ids
}

func (issues IssueList) loadPullRequests(e Engine) error {
	issuesIDs := issues.getPullIssueIDs()
	if len(issuesIDs) == 0 {
		return nil
	}

	pullRequestMaps := make(map[int64]*PullRequest, len(issuesIDs))
	rows, err := e.
		In("issue_id", issuesIDs).
		Rows(new(PullRequest))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pr PullRequest
		err = rows.Scan(&pr)
		if err != nil {
			return err
		}
		pullRequestMaps[pr.IssueID] = &pr
	}

	for _, issue := range issues {
		issue.PullRequest = pullRequestMaps[issue.ID]
	}
	return nil
}

func (issues IssueList) loadAttachments(e Engine) (err error) {
	if len(issues) == 0 {
		return nil
	}

	var attachments = make(map[int64][]*Attachment, len(issues))
	rows, err := e.Table("attachment").
		Join("INNER", "issue", "issue.id = attachment.issue_id").
		In("issue.id", issues.getIssueIDs()).
		Rows(new(Attachment))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var attachment Attachment
		err = rows.Scan(&attachment)
		if err != nil {
			return err
		}
		attachments[attachment.IssueID] = append(attachments[attachment.IssueID], &attachment)
	}

	for _, issue := range issues {
		issue.Attachments = attachments[issue.ID]
	}
	return nil
}

func (issues IssueList) loadComments(e Engine) (err error) {
	if len(issues) == 0 {
		return nil
	}

	var comments = make(map[int64][]*Comment, len(issues))
	rows, err := e.Table("comment").
		Join("INNER", "issue", "issue.id = comment.issue_id").
		In("issue.id", issues.getIssueIDs()).
		Rows(new(Comment))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var comment Comment
		err = rows.Scan(&comment)
		if err != nil {
			return err
		}
		comments[comment.IssueID] = append(comments[comment.IssueID], &comment)
	}

	for _, issue := range issues {
		issue.Comments = comments[issue.ID]
	}
	return nil
}

func (issues IssueList) loadTotalTrackedTimes(e Engine) (err error) {
	type totalTimesByIssue struct {
		IssueID int64
		Time    int64
	}
	if len(issues) == 0 {
		return nil
	}
	var trackedTimes = make(map[int64]int64, len(issues))

	var ids = make([]int64, 0, len(issues))
	for _, issue := range issues {
		if issue.Repo.IsTimetrackerEnabled() {
			ids = append(ids, issue.ID)
		}
	}

	// select issue_id, sum(time) from tracked_time where issue_id in (<issue ids in current page>) group by issue_id
	rows, err := e.Table("tracked_time").
		Select("issue_id, sum(time) as time").
		In("issue_id", ids).
		GroupBy("issue_id").
		Rows(new(totalTimesByIssue))
	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var totalTime totalTimesByIssue
		err = rows.Scan(&totalTime)
		if err != nil {
			return err
		}
		trackedTimes[totalTime.IssueID] = totalTime.Time
	}

	for _, issue := range issues {
		issue.TotalTrackedTime = trackedTimes[issue.ID]
	}
	return nil
}

// loadAttributes loads all attributes, expect for attachments and comments
func (issues IssueList) loadAttributes(e Engine) (err error) {
	if _, err = issues.loadRepositories(e); err != nil {
		return
	}

	if err = issues.loadPosters(e); err != nil {
		return
	}

	if err = issues.loadLabels(e); err != nil {
		return
	}

	if err = issues.loadMilestones(e); err != nil {
		return
	}

	if err = issues.loadAssignees(e); err != nil {
		return
	}

	if err = issues.loadPullRequests(e); err != nil {
		return
	}

	if err = issues.loadTotalTrackedTimes(e); err != nil {
		return
	}

	return nil
}

// LoadAttributes loads attributes of the issues, except for attachments and
// comments
func (issues IssueList) LoadAttributes() error {
	return issues.loadAttributes(x)
}

// LoadAttachments loads attachments
func (issues IssueList) LoadAttachments() error {
	return issues.loadAttachments(x)
}

// LoadComments loads comments
func (issues IssueList) LoadComments() error {
	return issues.loadComments(x)
}