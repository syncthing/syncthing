// Copyright (C) 2014 Audrius Butkevičius

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/AudriusButkevicius/cli"
	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/protocol"
)

func responseToBArray(response *http.Response) []byte {
	defer response.Body.Close()
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		die(err)
	}
	return bytes
}

func die(vals ...interface{}) {
	if len(vals) > 1 || vals[0] != nil {
		os.Stderr.WriteString(fmt.Sprintln(vals...))
		os.Exit(1)
	}
}

func wrappedHTTPPost(url string) func(c *cli.Context) {
	return func(c *cli.Context) {
		httpPost(c, url, "")
	}
}

func prettyPrintJSON(json map[string]interface{}) {
	writer := newTableWriter()
	remap := make(map[string]interface{})
	for k, v := range json {
		key, ok := jsonAttributeLabels[k]
		if !ok {
			key = firstUpper(k)
		}
		remap[key] = v
	}

	jsonKeys := make([]string, 0, len(remap))
	for key := range remap {
		jsonKeys = append(jsonKeys, key)
	}
	sort.Strings(jsonKeys)
	for _, k := range jsonKeys {
		var value string
		rvalue := remap[k]
		switch rvalue.(type) {
		case int, int16, int32, int64, uint, uint16, uint32, uint64, float32, float64:
			value = fmt.Sprintf("%.0f", rvalue)
		default:
			value = fmt.Sprint(rvalue)
		}
		if value == "" {
			continue
		}
		fmt.Fprintln(writer, k+":\t"+value)
	}
	writer.Flush()
}

func firstUpper(str string) string {
	for i, v := range str {
		return string(unicode.ToUpper(v)) + str[i+1:]
	}
	return ""
}

func newTableWriter() *tabwriter.Writer {
	writer := new(tabwriter.Writer)
	writer.Init(os.Stdout, 0, 8, 0, '\t', 0)
	return writer
}

func getMyID(c *cli.Context) string {
	response := httpGet(c, "system/status")
	data := make(map[string]interface{})
	json.Unmarshal(responseToBArray(response), &data)
	return data["myID"].(string)
}

func getConfig(c *cli.Context) config.Configuration {
	response := httpGet(c, "system/config")
	config := config.Configuration{}
	json.Unmarshal(responseToBArray(response), &config)
	return config
}

func setConfig(c *cli.Context, cfg config.Configuration) {
	body, err := json.Marshal(cfg)
	die(err)
	response := httpPost(c, "system/config", string(body))
	if response.StatusCode != 200 {
		die("Unexpected status code", response.StatusCode)
	}
}

func parseBool(input string) bool {
	val, err := strconv.ParseBool(input)
	if err != nil {
		die(input + " is not a valid value for a boolean")
	}
	return val
}

func parseInt(input string) int {
	val, err := strconv.ParseInt(input, 0, 64)
	if err != nil {
		die(input + " is not a valid value for an integer")
	}
	return int(val)
}

func parseUint(input string) int {
	val, err := strconv.ParseUint(input, 0, 64)
	if err != nil {
		die(input + " is not a valid value for an unsigned integer")
	}
	return int(val)
}

func parsePort(input string) int {
	port := parseUint(input)
	if port < 1 || port > 65535 {
		die(input + " is not a valid port\nExpected value between 1 and 65535")
	}
	return port
}

func validAddress(input string) {
	tokens := strings.Split(input, ":")
	if len(tokens) != 2 {
		die(input + " is not a valid value for an address\nExpected format <ip or hostname>:<port>")
	}
	matched, err := regexp.MatchString("^[a-zA-Z0-9]+([-a-zA-Z0-9.]+[-a-zA-Z0-9]+)?$", tokens[0])
	die(err)
	if !matched {
		die(input + " is not a valid value for an address\nExpected format <ip or hostname>:<port>")
	}
	parsePort(tokens[1])
}

func parseDeviceID(input string) protocol.DeviceID {
	device, err := protocol.DeviceIDFromString(input)
	if err != nil {
		die(input + " is not a valid device id")
	}
	return device
}
