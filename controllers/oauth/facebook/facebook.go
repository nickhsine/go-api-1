package facebook

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"twreporter.org/go-api/configs/constants"
	"twreporter.org/go-api/models"
	"twreporter.org/go-api/storage"
	"twreporter.org/go-api/utils"

	log "github.com/Sirupsen/logrus"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
)

var oauthConf *oauth2.Config

// Facebook ...
type Facebook struct {
	Storage storage.UserStorage
}

func initOauthConfig(location string) {
	if location == "" {
		location = "https://www.twreporter.org"
	}

	location = url.QueryEscape(location)
	redirectURL := utils.Cfg.OauthSettings.FacebookSettings.URL + "?location=" + location

	if oauthConf == nil {
		oauthConf = &oauth2.Config{
			ClientID:     utils.Cfg.OauthSettings.FacebookSettings.ID,
			ClientSecret: utils.Cfg.OauthSettings.FacebookSettings.Secret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"public_profile", "email"},
			Endpoint:     facebook.Endpoint,
		}
	} else {
		oauthConf.RedirectURL = redirectURL
	}
}

// BeginAuth redirects user to the Facebook Authentication
func (o Facebook) BeginAuth(c *gin.Context) {

	location := c.Query("location")
	initOauthConfig(location)
	URL, err := url.Parse(oauthConf.Endpoint.AuthURL)
	if err != nil {
		log.Error("Parse: ", err)
	}
	parameters := url.Values{}
	parameters.Add("client_id", oauthConf.ClientID)
	parameters.Add("scope", strings.Join(oauthConf.Scopes, " "))
	parameters.Add("redirect_uri", oauthConf.RedirectURL)
	parameters.Add("response_type", "code")
	parameters.Add("state", utils.Cfg.OauthSettings.FacebookSettings.Statestr)
	URL.RawQuery = parameters.Encode()
	url := URL.String()
	http.Redirect(c.Writer, c.Request, url, http.StatusTemporaryRedirect)
}

// Authenticate requests the user profile from Facebook
func (o Facebook) Authenticate(c *gin.Context) {
	log.Info("controllers.oauth.facebook.authenticate. OAuth type: ", constants.Facebook)
	location := c.Query("location")

	// get user data from Facebook
	fstring, err := getRemoteUserData(c.Request, c.Writer)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":      "unauthorized",
			"type":        constants.Facebook,
			"description": "Cannot get user data from Facebook.",
		})
		return
	}

	// decode user data returned by Facebook
	remoteOauth := models.OAuthAccount{
		Type:      constants.Facebook,
		AId:       utils.ToNullString(gjson.Get(fstring, "id").Str),
		Email:     utils.ToNullString(gjson.Get(fstring, "email").Str),
		Name:      utils.ToNullString(gjson.Get(fstring, "name").Str),
		FirstName: utils.ToNullString(gjson.Get(fstring, "first_name").Str),
		LastName:  utils.ToNullString(gjson.Get(fstring, "last_name").Str),
		Gender:    utils.GetGender(gjson.Get(fstring, "gender").Str),
		Picture:   utils.ToNullString(gjson.Get(fstring, "picture.data.url").Str),
	}

	log.WithFields(log.Fields{
		"Type": constants.Facebook,
		"AId":  remoteOauth.AId,
	}).Info("controllers.oauth.facebook.authenticate. OAuth Login")

	// find the OAuth user from the database
	matchUser, err := o.Storage.GetUserDataByOAuth(remoteOauth)
	// if the user doesn't exist, register the user automatically
	if err != nil {
		log.WithFields(log.Fields{
			"Type": constants.Facebook,
			"AId":  remoteOauth.AId,
			"Name": remoteOauth.Name,
		}).Info("controllers.oauth.facebook.authenticate. Create OAuth User")
		o.Storage.InsertUserByOAuth(remoteOauth)
	} else {
		// update existing OAuth data
		o.Storage.UpdateOAuthData(remoteOauth)
	}

	token, err := utils.RetrieveToken(matchUser.ID, matchUser.Privilege,
		matchUser.FirstName.String, matchUser.LastName.String, matchUser.Email.String)

	u, err := url.Parse(location)
	if err != nil {
		log.Error("controllers.oauth.facebook.authenticate_parse_location_error", err.Error())
		c.JSON(500, gin.H{"status": "Internal server error", "error": err.Error()})
		return
	}
	parameters := url.Values{}
	parameters.Add("token", token)
	u.RawQuery = parameters.Encode()
	url := u.String()
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// getRemoteUserData fetched user data from Facebook
func getRemoteUserData(r *http.Request, w http.ResponseWriter) (string, error) {

	oauthStateString := utils.Cfg.OauthSettings.FacebookSettings.Statestr
	loginPath := utils.Cfg.AppSettings.Path + "/login"

	// get Facebook OAuth Token
	state := r.FormValue("state")
	if state != oauthStateString {
		log.Warnf("controllers.oauth.facebook.getRemoteUserData. invalid oauth state, expected '%s', got '%s'\n", oauthStateString, state)
		http.Redirect(w, r, loginPath, http.StatusTemporaryRedirect)
		return "", models.NewAppError("OAuth state", "controllers.oauth.facebook", "Invalid oauth state", 500)
	}
	code := r.FormValue("code")

	token, err := oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Warnf("controllers.oauth.facebook.getRemoteUserData. oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, loginPath, http.StatusTemporaryRedirect)
		return "", models.NewAppError("Code exchange failed", "controllers.oauth.facebook", err.Error(), 500)
	}

	// get user data from Facebook
	resp, err := http.Get("https://graph.facebook.com/v2.8/me?fields=id,name,email,picture,birthday,first_name,last_name,gender&access_token=" +
		url.QueryEscape(token.AccessToken))
	if err != nil {
		log.Warnf("controllers.oauth.facebook.getRemoteUserData. Cannot get user info using Facebook API: %s\n", err)
		http.Redirect(w, r, loginPath, http.StatusTemporaryRedirect)
		return "", models.NewAppError("Cannot get user info using Facebook API", "controllers.oauth.facebook", err.Error(), 500)
	}
	defer resp.Body.Close()

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Warnf("controllers.oauth.facebook.getRemoteUserData. Error parsing Facebook user data: %s\n", err)
		http.Redirect(w, r, loginPath, http.StatusTemporaryRedirect)
		return "", models.NewAppError("Error parsing Facebook user data", "controllers.oauth.facebook", err.Error(), 500)
	}

	return string(response), nil
}