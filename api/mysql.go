package api

import (
	"io/ioutil"
	"net/http"
	"net/url"
)

var mysqlApi = "http://47.107.112.135:8089/back-server/device/update_device_status"

// UpdateToRemote updates device status in remote mysql
func UpdateToRemote(deviceID string, status string) string {
	urlValues := url.Values{}
	urlValues.Add("device_identifier", deviceID)
	urlValues.Add("device_status", status)
	resp, _ := http.PostForm(mysqlApi, urlValues)
	body, _ := ioutil.ReadAll(resp.Body)
	return string(body)
}
