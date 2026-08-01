package mail

import "fmt"

// GetAccountByName returns the single Mail.app account whose name exactly
// matches accountName.
func (c *Client) GetAccountByName(accountName string) (*Account, error) {
	accounts, err := c.GetAccountsJSON()
	if err != nil {
		return nil, err
	}

	return selectAccountByName(accounts, accountName)
}

func selectAccountByName(accounts []Account, accountName string) (*Account, error) {
	var match *Account
	for i := range accounts {
		if accounts[i].Name != accountName {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("select Mail.app account: %w", ErrAccountAmbiguous)
		}
		match = &accounts[i]
	}

	if match == nil {
		return nil, fmt.Errorf("select Mail.app account: %w", ErrAccountNotFound)
	}
	return match, nil
}
