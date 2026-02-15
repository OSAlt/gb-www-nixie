package domain

// SocialMedia represents a social media link with its associated platform name, URL, and display icon.
type SocialMedia struct {
	Platform string
	URL      string
	Icon     string
}

// MediaPost represents a social media post retrieved from an external media service,
// containing the post's unique identifier, image URL, caption text, and a link to the original post.
type MediaPost struct {
	ID       string
	ImageURL string
	Caption  string
	URL      string
}

// ContactMessage represents a contact form submission containing the sender's name, email, subject, and message body.
type ContactMessage struct {
	Name    string
	Email   string
	Subject string
	Message string
}
