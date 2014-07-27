package main

import (
	"bytes"
	"github.com/AudriusButkevicius/cli"
	"net/http"
	"strings"
)

type APIClient struct {
	httpClient http.Client
	endpoint   string
	apikey     string
	username   string
	password   string
	csrf       string
}

var instance *APIClient

func getClient(c *cli.Context) *APIClient {
	if instance != nil {
		return instance
	}
	endpoint := c.GlobalString("endpoint")
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}

	client := APIClient{
		httpClient: http.Client{},
		endpoint:   endpoint,
		apikey:     c.GlobalString("apikey"),
		username:   c.GlobalString("username"),
		password:   c.GlobalString("password"),
	}

	if client.apikey == "" {
		request, err := http.NewRequest("GET", client.endpoint, nil)
		die(err)
		response := client.handleRequest(request)
		for _, item := range response.Cookies() {
			if item.Name == "CSRF-Token" {
				client.csrf = item.Value
				goto csrffound
			}
		}
		die("Failed to get CSRF token")
	csrffound:
	}
	instance = &client
	return &client
}

func (client *APIClient) handleRequest(request *http.Request) *http.Response {
	if client.apikey != "" {
		request.Header.Set("X-API-Key", client.apikey)
	}
	if client.username != "" || client.password != "" {
		request.SetBasicAuth(client.username, client.password)
	}
	if client.csrf != "" {
		request.Header.Set("X-CSRF-Token", client.csrf)
	}

	response, err := client.httpClient.Do(request)
	die(err)

	if response.StatusCode == 404 {
		die("Invalid endpoint or API call")
	} else if response.StatusCode == 401 {
		die("Invalid username or password")
	} else if response.StatusCode == 403 {
		if client.apikey == "" {
			die("Invalid CSRF token")
		}
		die("Invalid API key")
	} else if response.StatusCode != 200 {
		body := strings.TrimSpace(string(responseToBArray(response)))
		if body != "" {
			die(body)
		}
		die("Unknown HTTP status returned: " + response.Status)
	}
	return response
}

func httpGet(c *cli.Context, url string) *http.Response {
	client := getClient(c)
	request, err := http.NewRequest("GET", client.endpoint+"/rest/"+url, nil)
	die(err)
	return client.handleRequest(request)
}

func httpPost(c *cli.Context, url string, body string) *http.Response {
	client := getClient(c)
	request, err := http.NewRequest("POST", client.endpoint+"/rest/"+url, bytes.NewBufferString(body))
	die(err)
	return client.handleRequest(request)
}
