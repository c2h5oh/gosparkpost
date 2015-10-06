// Package message_events interacts with the SparkPost Message Events API.
// https://www.sparkpost.com/api#/reference/message-events
package message_events

import (
	"encoding/json"
	"fmt"
	URL "net/url"
	"os"
	re "regexp"
	"strings"

	"github.com/SparkPost/go-sparkpost/api"
	"github.com/SparkPost/go-sparkpost/events"
)

// MessageEvents is your handle for the MessageEvents API.
type MessageEvents struct{ api.API }

// New gets a MessageEvents object ready to use with the specified config.
func New(cfg api.Config) (*MessageEvents, error) {
	m := &MessageEvents{}
	path := fmt.Sprintf("/api/v%d/message-events", cfg.ApiVersion)
	err := m.Init(cfg, path)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// Samples requests a list of example event data.
func (m MessageEvents) Samples(types *[]string) (*[]events.Event, error) {
	// append any requested event types to path
	var url string
	if types == nil {
		url = fmt.Sprintf("%s%s/events/samples", m.Config.BaseUrl, m.Path)
	} else {
		// validate types
		for _, etype := range *types {
			if !events.ValidEventType(etype) {
				return nil, fmt.Errorf("Invalid event type [%s]", etype)
			}
		}
		// break up the url into a net.URL object
		u, err := URL.Parse(fmt.Sprintf("%s%s/events/samples", m.Config.BaseUrl, m.Path))
		if err != nil {
			return nil, err
		}

		// get the query string object so we can modify it
		q := u.Query()
		// add the requested events and re-encode
		q.Set("events", strings.Join(*types, ","))
		u.RawQuery = q.Encode()
		url = u.String()
	}

	// Send off our request
	res, err := m.HttpGet(url)
	if err != nil {
		return nil, err
	}

	// Assert that we got a JSON Content-Type back
	if err = api.AssertJson(res); err != nil {
		return nil, err
	}

	// Get the Content
	bodyBytes, err := m.ReadBody(res)
	if err != nil {
		return nil, err
	}

	/*// DEBUG
	err = iou.WriteFile("./events.json", bodyBytes, 0644)
	if err != nil {
		return nil, err
	}
	*/

	// Parse expected response structure
	resMap := map[string][]*json.RawMessage{}
	err = json.Unmarshal(bodyBytes, &resMap)
	if err != nil {
		// FIXME: better error message
		return nil, err
	}

	// If the key "results" isn't present, something bad happened.
	results, ok := resMap["results"]
	if !ok {
		// FIXME: better error message
		return nil, fmt.Errorf("no results!")
	}

	return ParseEvents(results)
}

var typeMatch *re.Regexp = re.MustCompile("\"type\":\\s*\"(\\w+)\"")

func ParseEvents(jlist []*json.RawMessage) (*[]events.Event, error) {
	eventCount := len(jlist)
	elist := make([]events.Event, eventCount)

	i := 0
	for _, j := range jlist {
		// Coax the type out of the stringified event (sigh)
		tstr := typeMatch.FindStringSubmatch(string(*j))
		if len(tstr) < 2 {
			return nil, fmt.Errorf("ParseEvents didn't find an event type in:\n", string(*j))
		}

		e := events.EventForName(tstr[1])
		if e == nil {
			// TODO: log the offending event and continue
			fmt.Fprintf(os.Stderr, "unhandled event type [%s]\n", tstr[1])
			continue
		}

		// Parse event JSON into native object
		err := json.Unmarshal([]byte(*j), e)
		if err != nil {
			return nil, fmt.Errorf("error parsing [%s]: %s", tstr[1], err)
		}

		// Link object into the list/array we'll return
		elist[i] = e
		i++
	}

	rv := elist[:i]
	return &rv, nil
}
