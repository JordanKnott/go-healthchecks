package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/go-mail/mail"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"text/template"
	"time"
)

type SmtpConfig struct {
	Username string
	Password string
	Hostname string
	Port     int
}

type Config struct {
	SMTP SmtpConfig
}

// User blah
type User struct {
	Nicename string
	Email    string
	Enabled  bool
}

// Users blah
type Users struct {
	UserList []User
}

// Server blah
type Server struct {
	ID    int
	Label string
	URL   string
}

// Servers blah
type Servers struct {
	ServerList []Server
}

type AlertData struct {
	DownServers []ServerStatus
}

// ServerStatus blah
type ServerStatus struct {
	ID    int
	IsUp  bool
	Error string
	URL   string
	Date  time.Time
}

// LastRunDownServers list
type LastRunDownServers struct {
	DownServers  []ServerStatus
	RunTimeStart time.Time
	RunTimeEnd   time.Time
}

func GetServerStatus(targetServer Server) ServerStatus {
	if targetServer.ID == 196 {
		return ServerStatus{targetServer.ID, false, "404", targetServer.URL, time.Now()}
	}
	return ServerStatus{targetServer.ID, true, "", targetServer.URL, time.Now()}
}

func GetServerStatusReal(targetServer Server) ServerStatus {
	res, err := http.Get(targetServer.URL)
	if err != nil {
		return ServerStatus{
			targetServer.ID,
			false,
			err.Error(),
			targetServer.URL,
			time.Now(),
		}
	}
	if res.StatusCode != 200 {
		return ServerStatus{
			targetServer.ID,
			false,
			strconv.Itoa(res.StatusCode),
			targetServer.URL,
			time.Now(),
		}
	}
	return ServerStatus{
		targetServer.ID,
		true,
		"",
		targetServer.URL,
		time.Now(),
	}
}

// LoadConfig blah
func LoadConfig() Config {
	var config Config
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		panic(err)
	}
	return config
}

// LoadServers blah
func LoadServers() Servers {
	var servers Servers
	_, err := toml.DecodeFile("servers.toml", &servers)
	if err != nil {
		panic(err)
	}
	return servers
}

// LoadUsers blah
func LoadUsers() Users {
	var users Users
	_, err := toml.DecodeFile("users.toml", &users)
	if err != nil {
		panic(err)
	}
	return users
}

func LoadLastRunState() LastRunDownServers {
	if _, err := os.Stat("status.json"); os.IsNotExist(err) {
		return LastRunDownServers{[]ServerStatus{}, time.Now(), time.Now()}
	}
	lastRunStateRaw, err := ioutil.ReadFile("status.json")
	if err != nil {
		panic(err)
	}
	var lastRunState LastRunDownServers
	err = json.Unmarshal(lastRunStateRaw, &lastRunState)
	if err != nil {
		panic(err)
	}
	return lastRunState
}

func StatusExistsInLastRun(lastRun LastRunDownServers, targetStatus ServerStatus) bool {
	for _, status := range lastRun.DownServers {
		if status.ID == targetStatus.ID {
			return true
		}
	}
	return false
}

func SaveLastRunState(lastRunState LastRunDownServers) {
	// write new last run state
	b, err := json.MarshalIndent(lastRunState, "", "  ")
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile("status.json", b, 0644)
	if err != nil {
		panic(err)
	}
}

func GenerateAlertMessage(alertData AlertData) string {
	var tpl bytes.Buffer
	tmpl, err := ioutil.ReadFile("alert.html")
	if err != nil {
		panic(err)
	}
	t := template.New("Alert template")
	t, err = t.Parse(string(tmpl))
	err = t.Execute(&tpl, alertData)
	if err != nil {
		panic(err)
	}
	return tpl.String()
}

func main() {
	config := LoadConfig()
	lastRunState := LoadLastRunState()
	fmt.Println("Loading servers...")
	servers := LoadServers()
	currentRunState := []ServerStatus{}
	lastRunState.RunTimeStart = time.Now()
	for index, server := range servers.ServerList {
		fmt.Printf("[%d/%d] Checking %s (%d)...", index, len(servers.ServerList), server.URL, server.ID)
		serverStatus := GetServerStatusReal(server)
		if !serverStatus.IsUp {
			currentRunState = append(currentRunState, serverStatus)
		} else {
			fmt.Printf(" OK\n")
		}

	}
	lastRunState.RunTimeEnd = time.Now()
	fmt.Printf("%v\n", lastRunState.RunTimeEnd.Sub(lastRunState.RunTimeStart))

	shouldSend := false
	shouldSendList := []ServerStatus{}
	for _, status := range currentRunState {
		if !StatusExistsInLastRun(lastRunState, status) {
			if !shouldSend {
				shouldSend = true
			}
			shouldSendList = append(shouldSendList, status)
		}
	}

	alertData := AlertData{shouldSendList}
	alertMessage := GenerateAlertMessage(alertData)
	users := LoadUsers()
	if shouldSend {
		for _, user := range users.UserList {
			if !user.Enabled {
				continue
			}
			fmt.Printf("Sending alert email to %s\n", user.Email)
			m := mail.NewMessage()
			m.SetHeader("From", "monitor@drivendigital.us")
			m.SetHeader("To", user.Email)
			m.SetHeader("Subject", "IMPORTANT: Servers have gone down")
			m.SetBody("text/html", alertMessage)
			d := mail.NewDialer(config.SMTP.Hostname, config.SMTP.Port, config.SMTP.Username, config.SMTP.Password)
			d.StartTLSPolicy = mail.MandatoryStartTLS
			if err := d.DialAndSend(m); err != nil {
				panic(err)
			}
		}
	}
	lastRunState.DownServers = currentRunState
	fmt.Println("Saving current run state...")
	SaveLastRunState(lastRunState)
}
