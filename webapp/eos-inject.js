function addRepo(owner, repoName, price, callback) {
	alert("successfully created repo");

	tryCallback(callback, null, "successfully created repo");
}

function createLicense(to, repoName, callback) {
	alert("successfully created license");

	tryCallback(callback, null, "successfully created license");
}

const licStore = {};
function licensesBought(repoName, callback) {
	let licenses = licStore[repoName] | 0;
	licenses++;
	licStore[repoName] = licenses;
	tryCallback(callback, null, licenses);
}

function tryCallback(callback, err, result) {
	try {
		callback(err, result);
	}
	catch(e) {
		console.warn("error calling callback: " + e);
	}
}
