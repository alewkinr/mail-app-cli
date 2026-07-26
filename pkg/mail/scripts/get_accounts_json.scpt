function run(argv) {
	const mail = Application('Mail');
	const accounts = mail.accounts();
	const result = [];

	for (let i = 0; i < accounts.length; i++) {
		const acc = accounts[i];
		result.push({
			id: acc.id(),
			name: acc.name(),
			emailAddress: acc.emailAddresses().length > 0 ? acc.emailAddresses()[0] : '',
			userName: acc.userName(),
			enabled: acc.enabled()
		});
	}

	return JSON.stringify(result);
}
