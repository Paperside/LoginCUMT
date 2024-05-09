package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	// "net/url"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"

	"regexp"
	"time"
)

// Templates

const configFilePath string = "/usr/local/share/logincumt/login_config.json"

//const configFilePath string = "login_config.json"

var logFile *lumberjack.Logger
var config Config
var retErrInfoSheet []RetErrInfo

type Config struct {
	UserAccount   string `json:"user_account"`
	UserPassword  string `json:"user_password"`
	Operator      string `json:"operator"`
	LogFilePath   string `json:"log_file_path"`
	InfoSheetPath string `json:"info_sheet_path"`
}
type RetErrInfo struct {
	RetCode string `json:"ret_code"`
	RawInfo string `json:"raw_info"`
	InfoZH  string `json:"info_zh"`
	InfoEN  string `json:"info_en"`
}
type ResponseData struct {
	Result string `json:"result"`
	// result 1 登录成功 0 登录失败
	// a42.js:line:3254
	Msg string `json:"msg"`
	// base64 解码后匹配
	// a42.js:line:1903
	RetCode string `json:"ret_code"`
	// 返回值说明:0-成功 1帐户密码不对 2IP已经在线 3系统忙 4未知错误
	// 5-REQ_CHALLENGE失败 6-REQ_CHALLENGE超时 7-认证失败 8-认证超时 9-下线失败 10-下线超时 11-其他错误
	// a42.js:line:1891
}

// Components

func readConfig(path string) (Config, error) {
	//read local config file
	configFile, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err //open file error
	}
	//decode json data
	var config Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		return Config{}, err //decode json error
	}
	return config, nil
}

func readRetErrInfoSheet(path string) ([]RetErrInfo, error) {
	//read local config file
	configFile, err := os.ReadFile(path)
	if err != nil {
		return []RetErrInfo{}, err //open file error
	}
	var retErrInfoSheet []RetErrInfo
	//decode json data
	if err := json.Unmarshal(configFile, &retErrInfoSheet); err != nil {
		return []RetErrInfo{}, err //decode json error
	}
	return retErrInfoSheet, nil
}

func getIndexOfRetErrInfoSheet(rawinfo string) int {
	for i, v := range retErrInfoSheet {
		if v.RawInfo == rawinfo {
			return i
		}
	}
	return -1
}

func fetchURL(url string) (ResponseData, error) {
	//send GET
	resp, err := http.Get(url)
	if err != nil {
		return ResponseData{}, err //url fetching error
	}
	defer resp.Body.Close()

	//read response
	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || err != nil {
		if err == nil {
			return ResponseData{}, errors.New("request GET failed with response code " + fmt.Sprint(resp.StatusCode)) // response code error
		}
		return ResponseData{}, err //read resonse error
	}

	//transform response body
	re := regexp.MustCompile(`\((.*)\)$`)
	matches := re.FindSubmatch(body)
	if matches == nil || len(matches) < 2 {
		return ResponseData{}, errors.New("not expected response format") //response format error
	}

	//decode json data
	var data ResponseData
	if err := json.Unmarshal(matches[1], &data); err != nil {
		return ResponseData{}, err //decode json error
	}

	//decode msg base64
	if data.Result != "1" {
		decodedMsg, err := base64.StdEncoding.DecodeString(data.Msg)
		if err != nil {
			decodedMsg = []byte(data.Msg) // directly give raw msg if decode error
		}
		data.Msg = string(decodedMsg)
	}
	return data, nil
}

func resolveResponse(data ResponseData) (string, error) {
	if data.Result == "1" {
		return "CUMT校园网登录成功(￣▽￣)~*", nil
	} else if data.Result == "0" {
		var ret string = "状态：登录失败 "
		switch data.RetCode {
		case "1":
			{
				index := getIndexOfRetErrInfoSheet(data.Msg)
				if index == -1 {
					return ret + "信息：未知错误（没有找到对应的错误信息） Msg_EN: Unknown Error(No known infomation matched)", nil
				}
				return ret + "信息：" + retErrInfoSheet[index].InfoZH + " MSG_EN: " + retErrInfoSheet[index].InfoEN, nil
			}
		case "2":
			{
				return ret + "信息：您已登录校园网(～￣▽￣)～  Msg_EN: IP already online", nil
			}
		case "3":
			{
				return ret + "信息：系统繁忙，请稍后再试  Msg_EN: System busy, please try again later", nil
			}
		case "4":
			{
				return ret + "信息：未知错误  Msg_EN: Unknown Error", nil
			}
		case "5":
			{
				return ret + "信息: REQ_CHALLENGE失败  Msg_EN: REQ_CHALLENGE failed", nil
			}
		case "6":
			{
				return ret + "信息: REQ_CHALLENGE超时  Msg_EN: REQ_CHALLENGE timeout", nil
			}
		case "7":
			{
				return ret + "信息：认证失败  Msg_EN: Authentication failed", nil
			}
		case "8":
			{
				return ret + "信息：认证超时  Msg_EN: Authentication timeout", nil
			}
		case "9":
			{
				return ret + "信息：下线失败  Msg_EN: Offline failed", nil
			}
		case "10":
			{
				return ret + "信息：下线超时  Msg_EN: Offline timeout", nil
			}
		case "11":
			{
				return ret + "信息：其他错误  Msg_EN: Other error", nil
			}
		default:
			{
				return ret + "信息：未知错误  Msg_EN: Unknown Error", nil
			}
		}
	}
	return "未知错误", errors.New("resolve response Error")
}

func fetchCUMT(url string) {
	log.Info("The url trying to fetch: ", url)
	res, err := fetchURL(url)
	if err != nil {
		log.Error("Error fetching url: ", err)
		return
	}

	//resolve response and show
	info, err := resolveResponse(res)
	if err != nil {
		log.Error("Error resolving response: ", err)
		return
	}
	log.Info(info)
}

func scheduledCheckNetStatus() bool {
	url := "https://baidu.com"
	//send GET
	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	//check response code
	if resp.StatusCode != 200 {
		return false
	}
	return true
}

func reconnectCUMT(url string) {
	for {
		//check network status
		if scheduledCheckNetStatus() {
			log.Info("Network status check PASS, next check in 5 minutes...")
		} else {
			log.Info("Network status check FAIL, program will try to reconnect every 5 minutes...")
			fetchCUMT(url)
		}
		time.Sleep(time.Minute * 5)
	}
}

func scheduledFetchCUMT(url string) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, now.Location())
		if now.Unix() >= next.Unix() {
			next = next.Add(time.Hour * 24)
		}

		log.Info("Will try to Login CUMT at " + next.String() + "...")
		waitDuration := next.Sub(now)
		time.Sleep(waitDuration)
		fetchCUMT(url)

	}
}

// Main

func init() {
	//read config
	config_t, err := readConfig(configFilePath)
	if err != nil {
		log.SetLevel(log.ErrorLevel)
		log.Fatal("Error reading config file: ", err)
		return
	}
	config = config_t //Why do this?? because these functions's ret value is not our global variable predefined, even if their name is the same

	//start logging
	// logFile_t, err := os.OpenFile(config.LogFilePath+"app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	// if err != nil {
	// 	log.SetLevel(log.ErrorLevel)
	// 	log.Error("Error opening log file: ", err)
	// }
	logFile = &lumberjack.Logger{
		Filename:   config.LogFilePath + "app.log",
		MaxSize:    10, // megabytes
		MaxBackups: 3,
		MaxAge:     180, //days
	}
	logWriter := io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(logWriter)
	log.SetLevel(log.TraceLevel)

	//read ret_err_info_sheet
	retErrInfoSheet_t, err := readRetErrInfoSheet(config.InfoSheetPath)
	if err != nil {
		log.SetLevel(log.ErrorLevel)
		log.Error("Error reading ret_err_info_sheet file: ", err)
		return
	}
	retErrInfoSheet = retErrInfoSheet_t

	//initialization completed!
	log.Info("Initialization success!")
}

func main() {
	//pend closing opened files

	//catch signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	//fetch url
	url := "http://10.2.5.251:801/eportal/?c=Portal&a=login&login_method=1&user_account=" + config.UserAccount + "%40" + config.Operator + "&user_password=" + config.UserPassword

	go reconnectCUMT(url)
	go scheduledFetchCUMT(url)

	select {
	case <-sigChan:
		log.Info("Received signal, program will exit...")
	}
}
