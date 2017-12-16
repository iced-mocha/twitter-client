package handlers

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/garyburd/go-oauth/oauth"
	"github.com/gorilla/mux"
	"github.com/iced-mocha/shared/models"
	"github.com/iced-mocha/twitter-client/config"
	"github.com/iced-mocha/twitter-client/meta"
	"github.com/iced-mocha/twitter-client/session"
	gocache "github.com/patrickmn/go-cache"
)

const (
	tempCredKey = "tempCred"
	usernameKey = "username"
)

var oauthClient = oauth.Client{
	TemporaryCredentialRequestURI: "https://api.twitter.com/oauth/request_token",
	ResourceOwnerAuthorizationURI: "https://api.twitter.com/oauth/authorize",
	TokenRequestURI:               "https://api.twitter.com/oauth/access_token",
}

type CoreHandler struct {
	client *http.Client
	conf   *config.Config
	cache  *gocache.Cache
}

type twitterUser struct {
	Name            string `json:"name"`
	Handle          string `json:"screen_name"`
	ProfileImageURL string `json:"profile_image_url_https"`
}

type retweetStatus struct {
	Text       string      `json:"full_text"`
	User       twitterUser `json:"user"`
	Favourites int         `json:"favorite_count"`
}

type tweet struct {
	Retweets      int           `json:"retweet_count"`
	Favourites    int           `json:"favorite_count"`
	Text          string        `json:"full_text"`
	Timestamp     string        `json:"created_at"`
	ID            string        `json:"id_str"`
	IDNum         int64         `json:"id"`
	User          twitterUser   `json:"user"`
	RetweetStatus retweetStatus `json:"retweeted_status"`
}

func New(conf *config.Config) (*CoreHandler, error) {
	if conf == nil {
		return nil, errors.New("must initialize handler with non-nil config")
	}

	caCert, err := ioutil.ReadFile("/usr/local/etc/ssl/certs/core.crt")
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}

	oauthClient.Credentials.Token = conf.TwitterToken
	oauthClient.Credentials.Secret = conf.TwitterSecret

	h := &CoreHandler{client: client}
	h.cache = gocache.New(5*time.Minute, 10*time.Minute)
	h.conf = conf
	return h, nil
}

type AuthRequest struct {
	Token  string `json:"token"`
	Secret string `json:"secret"`
}

func (api *CoreHandler) GetIdentity(token, secret string) (*twitterUser, error) {
	creds := &oauth.Credentials{Token: token, Secret: secret}
	resp, err := oauthClient.Get(nil, creds, "https://api.twitter.com/1.1/account/verify_credentials.json", nil)
	if err != nil {
		log.Printf("Unable to complete request to twitter in GetIdentity: %v", err)
		return nil, fmt.Errorf("error completing request")
	}
	defer resp.Body.Close()

	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body from twitter in GetIdentity: %v", err)
		return nil, fmt.Errorf("error reading response body from twitter")
	}

	user := &twitterUser{}
	if err = json.Unmarshal(contents, user); err != nil {
		log.Printf("Unable to unmarshal body from twitter in GetIdentity: %v", err)
		return nil, fmt.Errorf("error unmarshaling response body from twitter")
	}

	return user, nil
}

func (api *CoreHandler) GetPosts(w http.ResponseWriter, r *http.Request) {
	username := mux.Vars(r)["userID"]
	log.Printf("Received request to GetPosts for user %v", username)
	pageToken := r.FormValue("continue")
	log.Printf("Received pagetoken %v in GetPosts", pageToken)

	contents, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Unable to read request body in GetPosts: %v", err)
		http.Error(w, "error reading request body", http.StatusInternalServerError)
		return
	}

	authRequest := &AuthRequest{}
	if err := json.Unmarshal(contents, authRequest); err != nil {
		log.Printf("Unable to marshal request body in GetPosts: %v", err)
		http.Error(w, "error parsing request body", http.StatusInternalServerError)
		return
	}

	pageMap := make(map[string][]byte)
	// Check our cache to avoid being ratelimited
	if pageMapI, exists := api.cache.Get(username); exists {
		// TODO fix this
		pageMap, _ = pageMapI.(map[string][]byte)

		if contents, exists := pageMap[pageToken]; exists {
			// We expect our posts to be stored as byte array
			log.Printf("Found requested page %v in cache", pageToken)
			w.Write(contents)
			return
		}
	}

	// Add values for twitter api
	form := make(url.Values)
	form["count"] = []string{"20"}
	form["tweet_mode"] = []string{"extended"}
	if pageToken != "" {
		form["max_id"] = []string{pageToken}
	}

	creds := &oauth.Credentials{Token: authRequest.Token, Secret: authRequest.Secret}
	resp, err := oauthClient.Get(nil, creds, "https://api.twitter.com/1.1/statuses/home_timeline.json", form)
	if err != nil {
		log.Printf("Unable to complete request to twitter in GetPosts: %v", err)
		http.Error(w, "error completing request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	contents, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Unable to read response body from twitter in GetPosts: %v", err)
		http.Error(w, "error reading response body from twitter", http.StatusInternalServerError)
		return
	}

	// We also need to marshal our contents to get the additional twitter meta data
	// Metatext will be a map of tweet id -> json string of tweet meta
	metaMap := meta.ParseMeta(contents)

	// First Marshal response in list of Twitter Posts
	twitterPosts := make([]tweet, 0)
	err = json.Unmarshal(contents, &twitterPosts)
	if err != nil {
		log.Printf("Unable to unmarshal body from twitter in GetPosts: %v", err)
		http.Error(w, "error unmarshaling response body from twitter", http.StatusInternalServerError)
		return
	}

	posts := []models.Post{}
	var maxID int64
	maxID = 1<<63 - 1

	for _, tweet := range twitterPosts {
		t, err := time.Parse(time.RubyDate, tweet.Timestamp)
		if err != nil {
			log.Printf("Unable to parse timestamp for tweet: %v - %v", tweet.ID, err)
		}

		// For pagination
		if tweet.IDNum < maxID {
			maxID = tweet.IDNum
		}

		text := tweet.Text
		imgURL := tweet.User.ProfileImageURL
		// Kind of hack to avoid making models struct too specific
		if tweet.RetweetStatus.Text != "" {
			text = "RT @" + tweet.RetweetStatus.User.Handle + ": " + tweet.RetweetStatus.Text
			imgURL = tweet.RetweetStatus.User.ProfileImageURL
		}

		favourites := tweet.Favourites
		if tweet.RetweetStatus.Favourites > favourites {
			favourites = tweet.RetweetStatus.Favourites
		}

		generic := models.Post{
			ID:          tweet.ID,
			Date:        t,
			Author:      tweet.User.Handle,
			DisplayName: tweet.User.Name,
			URL:         "https://twitter.com/" + tweet.User.Handle + "/status/" + tweet.ID,
			Platform:    "twitter",
			Score:       favourites + tweet.Retweets,
			Retweets:    tweet.Retweets,
			Favourites:  favourites,
			Title:       text,
			ProfileImg:  imgURL,
			Meta:        metaMap[tweet.ID],
		}
		posts = append(posts, generic)
	}

	nextURI := fmt.Sprintf("/v1/%v/posts?continue=%v", username, maxID-1)
	log.Printf("Constructed next URI: %v", nextURI)
	clientResp := models.ClientResp{
		Posts:   posts,
		NextURL: nextURI,
	}

	contents, err = json.Marshal(clientResp)
	if err != nil {
		log.Printf("Unable to marshal posts into json: %v", err)
		http.Error(w, "error marshaling posts into json", http.StatusInternalServerError)
		return
	}

	// Update our value into the cache
	pageMap[pageToken] = contents
	api.cache.Set(username, pageMap, gocache.DefaultExpiration)
	w.Write(contents)
}

func (api *CoreHandler) AuthorizeCallback(w http.ResponseWriter, r *http.Request) {
	log.Println("Reaceived callback from Reddit oauth")
	s := session.Get(r.FormValue("oauth_token"))
	tempCred, ok := s[tempCredKey].(*oauth.Credentials)
	if tempCred == nil || !ok {
		http.Error(w, "Unknown oauth_token.", 500)
		return
	}

	username, ok := s[usernameKey].(string)
	if !ok {
		http.Error(w, "Invalid username.", http.StatusInternalServerError)
		return
	}

	authToken, _, err := oauthClient.RequestToken(nil, tempCred, r.FormValue("oauth_verifier"))
	if err != nil {
		http.Error(w, "Error getting request token, "+err.Error(), 500)
		return
	}
	delete(s, tempCredKey)

	user, err := api.GetIdentity(authToken.Token, authToken.Secret)
	if err != nil {
		http.Error(w, "Unable to get identity for user "+err.Error(), 500)
		return
	}

	go api.PostTwitterSecrets(authToken.Token, authToken.Secret, username, user.Handle)

	http.Redirect(w, r, api.conf.FrontendURL, 302)
}

func (api *CoreHandler) PostTwitterSecrets(token, secret, userID, twitterUsername string) {
	// Post the bearer token to be saved in core
	log.Printf("Preparing to store twitter account in core for user: %v", userID)
	// TODO: Get twitter handler redditUsername, err := api.GetIdentity(bearerToken)

	jsonStr := []byte(fmt.Sprintf(`{ "type": "twitter", "username": "%v", "token": "%v", "secret": "%v"}`, twitterUsername, token, secret))
	req, err := http.NewRequest(http.MethodPost, api.conf.CoreURL+"/v1/users/"+userID+"/authorize/twitter", bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Printf("Unable to post bearer token for user: %v - %v", userID, err)
		return
	}

	// TODO: add retry logic
	resp, err := api.client.Do(req)
	if err != nil {
		log.Printf("Unable to complete request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Could not post reddit data to core: %v", err)
	}
}

// This function initiates a request from Reddit to authorize via oauth
// GET /v1/{userID}/authorize
func (api *CoreHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	username := mux.Vars(r)["userID"]

	tempCred, err := oauthClient.RequestTemporaryCredentials(nil, api.conf.CallbackURL, nil)
	if err != nil {
		http.Error(w, "Error getting temp cred, "+err.Error(), 500)
		return
	}

	s := session.Get(tempCred.Token)
	s[tempCredKey] = tempCred
	s[usernameKey] = username
	session.Save(tempCred.Token, s)

	fmt.Printf("Temp creds: %v", tempCred)
	http.Redirect(w, r, oauthClient.AuthorizationURL(tempCred, nil), 302)
}
