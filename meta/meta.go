package meta

import (
	"encoding/json"
	"log"
)

type Hashtag struct {
	TextWithHash string `json:"text_with_hash"`
	Text         string `json:"text"`
	Link         string `json:"link"`
}

type MediaSize struct {
	Width  int `json:"w"`
	Height int `json:"h"`
}

type Media struct {
	Type       string               `json:"type"`
	MediaURL   string               `json:"media_url_https"`
	URL        string               `json:"url"`
	MediaSizes map[string]MediaSize `json:"sizes"`
	VideoInfo  interface{}          `json:"video_info"`
}

type Mention struct {
	Handle string `json:"screen_name"`
	Name   string `json:"name"`
	Link   string `json:"link"`
}

type URL struct {
	URL        string `json:"expanded_url"`
	DisplayURL string `json:"display_url"`
}

type Entity struct {
	Hashtags []Hashtag `json:"hashtags"`
	Medias   []Media   `json:"media"`
	Mentions []Mention `json:"user_mentions"`
	URLs     []URL     `json:"urls"`
}

type TweetMeta struct {
	Entities Entity `json:"extended_entities"`
	ID       string `json:"id_str"`
}

func buildMentions(mentions []Mention) []Mention {
	for i, m := range mentions {
		m.Link = "https.//twitter.com/" + m.Handle
		mentions[i] = m
	}
	return mentions
}

func buildHashtags(hashtags []Hashtag) []Hashtag {
	for i, h := range hashtags {
		h.TextWithHash = "#" + h.Text
		h.Link = "https://twitter.com/hashtag/" + h.Text
		hashtags[i] = h
	}

	return hashtags
}

// Dont fail if we cant produce meta, its not a game break
// The rest will still work
func ParseMeta(contents []byte) map[string]string {
	metas := []TweetMeta{}
	err := json.Unmarshal(contents, &metas)
	if err != nil {
		log.Printf("Unable to parse tweet meta: %v", err)
		return nil
	}

	var metaJSON []byte
	metaMap := make(map[string]string)
	for _, m := range metas {
		m.Entities.Hashtags = buildHashtags(m.Entities.Hashtags)
		m.Entities.Mentions = buildMentions(m.Entities.Mentions)

		if metaJSON, err = json.Marshal(m); err != nil {
			continue
		}
		metaMap[m.ID] = string(metaJSON)
	}

	return metaMap
}
