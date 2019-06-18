package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/go-mail/mail"
	homedir "github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

// HealthcheckConfig contains general options
type HealthcheckConfig struct {
	TryWithBackoff bool
}

// DebugConfig contains debug options
type DebugConfig struct {
	MockFetch bool
}

// Config contains all of application configuration information
type Config struct {
	SMTP        SMTPConfig
	Healthcheck HealthcheckConfig
	Debug       DebugConfig
}

// User blah
type User struct {
	Nicename string
	Email    string
	Enabled  bool
}

type WebsiteFetcher interface {
	Fetch(target Server) ServerStatus
}

type FetchWithBackoff struct{}

type MockFetch struct{}

// Users blah
type Users struct {
	UserList []User
}

// Server blah
type Server struct {
	ID       int
	Label    string
	URL      string
	Install  string
	Protocol string
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
func (f MockFetch) Fetch(targetServer Server) ServerStatus {
	if targetServer.ID == 196 {
		return ServerStatus{targetServer.ID, false, "404", targetServer.URL, time.Now()}
	}
	return ServerStatus{targetServer.ID, true, "", targetServer.URL, time.Now()}
}

func (f FetchWithBackoff) Fetch(targetServer Server) ServerStatus {
	currentCheck := 1
	maxChecks := 3
	var lastStatusCode int
	var err error
	for currentCheck < maxChecks {
		log.WithFields(log.Fields{
			"current_check": currentCheck,
			"target":        targetServer.Protocol + targetServer.URL,
		}).Info("fetching server status")
		resp, newErr := http.Get(targetServer.Protocol + targetServer.URL)
		if newErr != nil {
			log.WithFields(log.Fields{
				"error": err.Error(),
			}).Error("an error occurred while fetching site")
			err = newErr
		} else {
			err = nil
		}
		if resp.StatusCode == 200 {
			return ServerStatus{
				targetServer.ID,
				true,
				"",
				targetServer.URL,
				time.Now(),
			}
		}
		time.Sleep(1 * time.Minute)
		lastStatusCode = resp.StatusCode
	}
	log.WithFields(log.Fields{
		"status_code": lastStatusCode,
	}).Error("non 200 status code")
	if err != nil {
		return ServerStatus{
			targetServer.ID,
			false,
			err.Error(),
			targetServer.URL,
			time.Now(),
		}
	}
	return ServerStatus{
		targetServer.ID,
		false,
		strconv.Itoa(lastStatusCode),
		targetServer.URL,
		time.Now(),
	}
}

func IsWebsiteUp(url string) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return errors.New("Non 200 status code: " + strconv.Itoa(res.StatusCode))
	}
	return nil

}

func GetWebsiteStatus(fetcher WebsiteFetcher, target Server) ServerStatus {
	return fetcher.Fetch(target)
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

func init() {
	// Log as JSON instead of the default ASCII formatter.
	textFormatter := &log.TextFormatter{}
	textFormatter.FullTimestamp = true
	log.SetFormatter(textFormatter)

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	// log.SetLevel(log.WarnLevel)
}
func checkSites() {
	log.Info("loading config")
	// App setup
	config := LoadConfig()
	lastRunState := LoadLastRunState()
	servers := LoadServers()

	var fetcher WebsiteFetcher

	if config.Debug.MockFetch {
		fetcher = MockFetch{}
	} else {
		fetcher = FetchWithBackoff{}
	}

	// Check servers
	currentRunState := []ServerStatus{}
	lastRunState.RunTimeStart = time.Now()
	for index, server := range servers.ServerList {
		log.WithFields(log.Fields{
			"index":     index,
			"total":     len(servers.ServerList),
			"url":       server.URL,
			"server_id": server.ID,
		}).Info("checking server status")
		serverStatus := GetWebsiteStatus(fetcher, server)
		if !serverStatus.IsUp {
			currentRunState = append(currentRunState, serverStatus)
			log.WithFields(log.Fields{
				"error": serverStatus.Error,
			}).Error("target server has an error")
		} else {
			log.Info("server is okay")
		}

	}
	lastRunState.RunTimeEnd = time.Now()
	log.WithFields(log.Fields{
		"duration": lastRunState.RunTimeEnd.Sub(lastRunState.RunTimeStart),
	}).Info("server check complete")

	// Check if any down servers should be sent to users
	shouldSend := false
	shouldSendList := []ServerStatus{}
	for _, status := range currentRunState {
		log.WithFields(log.Fields{
			"status": status,
		}).Info("comparing status against last run")
		if !StatusExistsInLastRun(lastRunState, status) {
			if !shouldSend {
				shouldSend = true
			}
			log.WithFields(log.Fields{
				"status": status,
			}).Info("should be sent")
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
	SaveLastRunState(lastRunState)
}

var cfgFile string
var serversFile string
var statusFile string
var verbose bool

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.servermon.yaml)")
	rootCmd.PersistentFlags().StringVarP(&serversFile, "servers", "s", "servers.toml", "")
	rootCmd.PersistentFlags().StringVarP(&serversFile, "servers", "s", "status.toml", "")
	rootCmd.PersistentFlags().Bool("verbose", true, "show logs")
	viper.BindPFlag("serversFile", rootCmd.PersistentFlags().Lookup("serversFile"))
	viper.BindPFlag("statusFile", rootCmd.PersistentFlags().Lookup("statusFile"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

func initConfig() {
	// Don't forget to read config either from cfgFile or from home directory!
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".servermon")
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("Can't read config:", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "servermon",
	Short: "Check server health",
	Long:  "check server health",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
