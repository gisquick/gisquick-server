package domain

type User struct {
	Username        string  `json:"username"`
	Email           string  `json:"email"`
	FirstName       string  `json:"first_name"`
	LastName        string  `json:"last_name"`
	IsSuperuser     bool    `json:"is_superuser"`
	IsAuthenticated bool    `json:"-"`
	IsGuest         bool    `json:"is_guest"`
	Profile         Profile `json:"profile,omitempty"`
}

func AccountToUser(account Account) User {
	return User{
		Username:        account.Username,
		Email:           account.Email,
		FirstName:       account.FirstName,
		LastName:        account.LastName,
		IsSuperuser:     account.Superuser,
		IsGuest:         false,
		IsAuthenticated: true,
		Profile:         account.Profile,
	}
}
