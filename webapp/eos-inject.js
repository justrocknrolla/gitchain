// EOS-JS API
let eos;

// initialize scatter and EOS
function initScatter() {
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
		window.scatter = null;

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
	}
	catch(e) {
		console.warn("EOS initialization failed: " + e);
	}
}

function addRepo(owner, repoName, price, callback) {
	// init scatter if required
	initScatter();

	let e = null;
	try {
		eos.contract('licensing').then(contract => {
			contract.addrepo(...arguments);
		});
	}
	catch(e) {
		console.warn("error executing addRepo: " + e);
	}
	tryCallback(callback, e, e? null: "successfully created repo");
}

function createLicense(to, repoName, callback) {
	// init scatter if required
	initScatter();

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
function licensesBought(repoName, callback) {
	// init scatter if required
	initScatter();

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
