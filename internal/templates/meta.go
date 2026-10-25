package templates

import (
	"encoding/json"
)

// siteURL is the canonical origin used for Open Graph, Twitter and JSON-LD
// metadata. Update it here if the production domain changes.
const siteURL = "https://nixiepixel.com"

// socialLink is one external profile. It powers the header/footer icon list and
// the JSON-LD sameAs array, so the URLs only live in one place.
type socialLink struct {
	Name string
	URL  string
	Icon string
}

var socialLinks = []socialLink{
	{Name: "YouTube", URL: "https://www.youtube.com/user/nixiedoeslinux", Icon: "fab fa-youtube"},
	{Name: "Twitter", URL: "https://twitter.com/NixiePixel", Icon: "fab fa-twitter"},
	{Name: "Facebook", URL: "https://www.facebook.com/nixiepixel", Icon: "fab fa-facebook-f"},
	{Name: "Instagram", URL: "https://www.instagram.com/nixie.pixel", Icon: "fab fa-instagram"},
	{Name: "Twitch", URL: "https://www.twitch.tv/nixiepixel", Icon: "fab fa-twitch"},
	{Name: "TikTok", URL: "https://www.tiktok.com/@nixiepixel", Icon: "fab fa-tiktok"},
	{Name: "Patreon", URL: "https://www.patreon.com/nixiepixel", Icon: "fab fa-patreon"},
	{Name: "GitHub", URL: "https://github.com/nixiepixel", Icon: "fab fa-github"},
	{Name: "Keybase", URL: "https://keybase.io/nixiepixel", Icon: "fab fa-keybase"},
}

// canonicalURL turns a route path into an absolute URL for canonical, Open
// Graph and Twitter tags.
func canonicalURL(path string) string {
	if path == "" {
		path = "/"
	}
	return siteURL + path
}

type personSchema struct {
	Context       string   `json:"@context"`
	Type          string   `json:"@type"`
	Name          string   `json:"name"`
	AlternateName string   `json:"alternateName"`
	URL           string   `json:"url"`
	JobTitle      string   `json:"jobTitle"`
	Description   string   `json:"description"`
	SameAs        []string `json:"sameAs"`
}

func sameAsURLs() []string {
	urls := make([]string, 0, len(socialLinks))
	for _, link := range socialLinks {
		urls = append(urls, link.URL)
	}
	return urls
}

// personJSONLD renders static structured data. It contains no user input, so it
// is safe to inject into a script tag without HTML escaping.
func personJSONLD() string {
	payload, err := json.Marshal(personSchema{
		Context:       "https://schema.org",
		Type:          "Person",
		Name:          "Nixie Pixel",
		AlternateName: "NixiePixel",
		URL:           siteURL,
		JobTitle:      "Technology creator and open-source advocate",
		Description:   "Linux, gaming and open-source creator with 410,000+ followers across platforms.",
		SameAs:        sameAsURLs(),
	})
	if err != nil {
		return "{}"
	}
	return string(payload)
}

// personJSONLDScript wraps the structured data in its script element. The
// payload is generated from constants, so it contains no user input.
func personJSONLDScript() string {
	return `<script type="application/ld+json">` + personJSONLD() + `</script>`
}
