package domain

type SocialMedia struct {
	Platform string
	URL      string
	Icon     string
}

type MediaPost struct {
	ID       string
	ImageURL string
	Caption  string
	URL      string
}

type ContactMessage struct {
	Name    string
	Email   string
	Subject string
	Message string
}
