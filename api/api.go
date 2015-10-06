// Package api provides structures and functions used by other SparkPost API packages.
package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	certifi "github.com/certifi/gocertifi"
)

// Config includes all information necessary to make an API request.
type Config struct {
	BaseUrl    string
	ApiKey     string
	ApiVersion int
}

// NewConfig builds a Config object using the provided map.
func NewConfig(m map[string]string) (*Config, error) {
	c := &Config{}

	if baseurl, ok := m["baseurl"]; ok {
		c.BaseUrl = baseurl
	} else {
		return nil, fmt.Errorf("BaseUrl is required for api config")
	}

	if apikey, ok := m["apikey"]; ok {
		c.ApiKey = apikey
	} else {
		return nil, fmt.Errorf("ApiKey is required for api config")
	}

	if apiver, ok := m["apiver"]; ok {
		var err error
		c.ApiVersion, err = strconv.Atoi(apiver)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("ApiVer is required for api config")
	}

	return c, nil
}

// Response contains information about the last HTTP response.
// Helpful when an error message doesn't necessarily give the complete picture.
type Response struct {
	HTTP    *http.Response
	Body    []byte
	Results map[string]string `json:"results,omitempty"`
	Errors  []Error           `json:"errors,omitempty"`
}

// API exists to be embedded in other API objects, to enable transparent method forwarding.
type API struct {
	Path     string
	Config   *Config
	Client   *http.Client
	Response *Response
}

// Error mirrors the error format returned by SparkPost APIs.
type Error struct {
	Message     string `json:"message"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Part        string `json:"part,omitempty"`
	Line        int    `json:"line,omitempty"`
}

// Init sets each API's path and pulls together everything necessary to make an API request.
// Caller may provide their own http.Client by setting it in the provided API object.
func (api *API) Init(cfg Config, path string) error {
	api.Config = &cfg
	api.Path = path

	if api.Client == nil {
		// Ran into an issue where USERTrust was not recognized on OSX.
		// The rest of this block was the fix.

		// load Mozilla cert pool
		pool, err := certifi.CACerts()
		if err != nil {
			return err
		}

		// configure transport using Mozilla cert pool
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		}

		// configure http client using transport
		api.Client = &http.Client{Transport: transport}
	}

	return nil
}

// HttpPost sends a Post request with the provided JSON payload to the specified url.
// Query params are supported via net/url - roll your own and stringify it.
// Authenticate using the configured API key.
func (api *API) HttpPost(url string, data []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", api.Config.ApiKey)
	return api.Client.Do(req)
}

// HttpGet sends a Get request to the specified url.
// Query params are supported via net/url - roll your own and stringify it.
// Authenticate using the configured API key.
func (api *API) HttpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", api.Config.ApiKey)
	return api.Client.Do(req)
}

// HttpDelete sends a Delete request to the provided url.
// Query params are supported via net/url - roll your own and stringify it.
// Authenticate using the configured API key.
func (api *API) HttpDelete(url string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", api.Config.ApiKey)
	return api.Client.Do(req)
}

// ReadBody is a convenience wrapper that returns the response body.
func (api *API) ReadBody(res *http.Response) ([]byte, error) {
	// Calls 2+ to this function for the same http.Response will now DWIM
	if api.Response != nil && api.Response.Body != nil {
		return api.Response.Body, nil
	}

	defer res.Body.Close()
	bodyBytes, err := ioutil.ReadAll(res.Body)
	if api.Response == nil {
		api.Response = new(Response)
	}
	api.Response.Body = bodyBytes
	return bodyBytes, err
}

// ParseApiResponse pulls info from JSON http responses into api.Response object.
// It's helpful to call api.AssertJson before calling this function.
func (api *API) ParseResponse(res *http.Response) error {
	body, err := api.ReadBody(res)
	if err != nil {
		return err
	}

	if api.Response == nil {
		api.Response = new(Response)
	}
	err = json.Unmarshal(body, api.Response)
	if err != nil {
		return fmt.Errorf("Failed to parse API response: [%s]\n%s", err, string(body))
	}
	api.Response.HTTP = res

	return nil
}

// AssertObject asserts that the provided variable is a map[string]something.
// The string parameter is used to customize the generated error message.
func AssertObject(obj interface{}, label string) error {
	// List of handled types from here:
	// http://golang.org/pkg/encoding/json/#Unmarshal
	switch objVal := obj.(type) {
	case map[string]interface{}:
		// auto-parsed nested json object
	case map[string]bool:
		// user-provided json literal (convenience)
	case map[string]float64:
		// user-provided json literal (convenience)
	case map[string]string:
		// user-provided json literal (convenience)
	case map[string][]interface{}:
		// user-provided json literal (convenience)
	case map[string]map[string]interface{}:
		// user-provided json literal (convenience)
	default:
		return fmt.Errorf("expected key/val pairs for %s, got [%s]", label, reflect.TypeOf(objVal))
	}
	return nil
}

var jctype string = "application/json"

// AssertJson returns an error if the provided HTTP response isn't JSON.
func AssertJson(res *http.Response) error {
	if res == nil {
		return fmt.Errorf("AssertJson got nil http.Response")
	}
	ctype := res.Header.Get("Content-Type")
	// allow things like "application/json; charset=utf-8" in addition to the bare content type
	if !(strings.EqualFold(ctype, jctype) || strings.HasPrefix(ctype, jctype)) {
		return fmt.Errorf("Expected json, got [%s] with code %d", ctype, res.StatusCode)
	}
	return nil
}

// PrettyError returns a human-readable error message for common http errors returned by the API.
// The string parameters are used to customize the generated error message
// (example: noun=template, verb=create).
func PrettyError(noun, verb string, res *http.Response) error {
	if res.StatusCode == 404 {
		return fmt.Errorf("%s does not exist, %s failed.", noun, verb)
	} else if res.StatusCode == 401 {
		return fmt.Errorf("%s %s failed, permission denied. Check your API key.", noun, verb)
	} else if res.StatusCode == 403 {
		// This is what happens if an endpoint URL gets typo'd. (dgray 2015-09-14)
		return fmt.Errorf("%s %s failed. Are you using the right API path?", noun, verb)
	}
	return nil
}
