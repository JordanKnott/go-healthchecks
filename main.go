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

// SMTPConfig contains a SMTP server's details
type SMTPConfig struct {
	Username string
	Password string
	Hostname string
	Port     int
}

// Config contains all of application configuration information
type Config struct {
	SMTP SMTPConfig
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

// AlertData is a list of server states that should be sent to users
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

// GetServerStatus is a testing function that returns a mock ServerStatus for a Server
func GetServerStatus(targetServer Server) ServerStatus {
	if targetServer.ID == 196 {
		return ServerStatus{targetServer.ID, false, "404", targetServer.URL, time.Now()}
	}
	return ServerStatus{targetServer.ID, true, "", targetServer.URL, time.Now()}
}

// GetServerStatusReal tries to load a url for a given server and returns an appropriate ServerStatus
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

// LoadLastRunState retrieves the last list of down servers to prevent sending an alert more than once
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

// StatusExistsInLastRun checks if the given server exists in the last run based on ID
func StatusExistsInLastRun(lastRun LastRunDownServers, targetStatus ServerStatus) bool {
	for _, status := range lastRun.DownServers {
		if status.ID == targetStatus.ID {
			return true
		}
	}
	return false
}

// SaveLastRunState writes the new lastRunState to status.json
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

// GenerateAlertMessage uses the go template alert.html to generate the alert email sent to users
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
	fmt.Println("Loading servers...")
	// App setup
	config := LoadConfig()
	lastRunState := LoadLastRunState()
	servers := LoadServers()

	// Check servers
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
	fmt.Printf("Server Check Duration: %v\n", lastRunState.RunTimeEnd.Sub(lastRunState.RunTimeStart))

	// Check if any down servers should be sent to users
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

	// Send out alerts if needed
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
	// Save new run state
	lastRunState.DownServers = currentRunState
	fmt.Println("Saving current run state...")
	SaveLastRunState(lastRunState)
}
