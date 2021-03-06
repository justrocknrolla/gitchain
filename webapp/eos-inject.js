// EOS-JS API
let eos;

// initialize scatter and EOS
function initScatter(callback) {
	if(eos) {
		// no need to reinitialize
		return;
	}
	try {
		// Scatter will now be available from the window scope.
		// At this stage the connection to Scatter from the application is
		// already encrypted.
		const scatter = window.scatter;

		// take this off the window once we have a reference to it
		// window.scatter = null;

		// connection settings
		const network = {
			blockchain: 'eos',
			host: 'localhost', // ( or null if endorsed chainId )
			port: 8888, // ( or null if defaulting to 80 )
			chainId: 1 || 'abcd', // Or null to fetch automatically ( takes longer )
		};

		// Set up any extra options we want to use eosjs with.
		const eosOptions = {};

		// Get a reference to an 'Eosjs' instance with a Scatter signature provider.
		eos = scatter.eos(network, EosApi.Localnet, eosOptions, 'https');

		// check scatter integration
		scatter.getIdentity().then(function() {
			scatter.forgetIdentity().then(function() {
				console.log("scatter integration check complete");
				tryCallback(callback, null, "scatter integration check complete");
			}).catch(function(err) {
				console.warn(err);
				tryCallback(callback, err, null);
			});

		}).catch(function(err) {
			console.warn(err);
			tryCallback(callback, err, null);
		});
	}
	catch(e) {
		console.warn("EOS initialization failed: " + e);
	}
}

function addRepo(repoName, price, callback) {
	// re-init scatter
	eos = null;
	initScatter(callback);

	let e = null;
	try {
		eos.contract('licensing').then(contract => {
			contract.addrepo(...arguments);
		});
	}
	catch(e) {
		console.warn("error executing addRepo: " + e);
	}
	// tryCallback(callback, e, e? null: "successfully created repo");
}

function createLicense(repoName, callback) {
	// re-init scatter
	eos = null;
	initScatter(function(err, result) {
		if(err) {
			return;
		}
        showLicenses(licensesBought(repoName));
	});

	let e = null;
	try {
		eos.contract('licensing').then(contract => {
			contract.createlicense(...arguments);
		});
	}
	catch(e) {
		console.warn("error executing createLicense: " + e);
	}
	tryCallback(callback, e, e? null: "successfully created license");
}

// method stub for now, to be implemented later
const licStore = {};

function licensesBought(repoName) {
	// init scatter if required
	// initScatter();

	let licenses = licStore[repoName] || 0;
	licenses++;
	licStore[repoName] = licenses;
	return licenses;
}

function showLicenses(n) {
	$('#licensesCount').text(n)
}

showLicenses(0);

function tryCallback(callback, err, result) {
	try {
		callback(err, result);
	}
	catch(e) {
		console.warn("error calling callback: " + e);
	}
}
