package model

type FeedResult struct {
	ID         string `json:"id"`
	PublicURL  string `json:"public_url"`
	Title      string `json:"title,omitempty"`
	FetchError string `json:"fetch_error,omitempty"`
	Feed       *Feed  `json:"feed,omitempty"`
}
