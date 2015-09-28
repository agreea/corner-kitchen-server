package main

import (
	"bitbucket.org/ckvist/twilio/twirest"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var server_config Config

var http_server http.Server

/*
 *  CLI Arguments
 */

// Config file location
var config_file string

const config_file_default = "cornerd.gcfg"
const config_file_usage = "Specify configuration file"

// Log to stderr for debugging
var config_log_stderr bool

const config_log_stderr_default = false
const config_log_stderr_usage = "Log to stderr instead of the specified logfiles"

func init() {
	flag.StringVar(&config_file, "config", config_file_default, config_file_usage)
	flag.StringVar(&config_file, "c", config_file_default, config_file_usage+" (shorthand)")

	flag.BoolVar(&config_log_stderr, "stderr", config_log_stderr_default, config_log_stderr_usage)
}

var DB_CONN *sql.DB
var SESSION_MGR *SessionManager
var TWILIO_MESSAGEQUEUE chan *SMS

func initHelpers() {
	var err error
	DB_CONN, err = sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewSessionManager", "Failed to open database:", err)
	}
	SESSION_MGR = NewSessionManager(&server_config)
	twilio_client := twirest.NewClient(server_config.Twilio.SID, server_config.Twilio.Token)
	TWILIO_MESSAGEQUEUE = make(chan *SMS, 100)
	go workTwilioQueue(twilio_client, TWILIO_MESSAGEQUEUE)
}

func initApiServer() {
	bind_address := server_config.API.BindAddress + ":" + server_config.API.BindPort

	api_handler := NewApiHandler(&server_config)

	http_server = http.Server{
		Addr:           bind_address,
		Handler:        api_handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	api_handler.AddServlet("/version", NewVersionServlet())
	api_handler.AddServlet("/user", NewUserServlet(&server_config, SESSION_MGR, TWILIO_MESSAGEQUEUE))
	api_handler.AddServlet("/truck", NewTruckServlet(&server_config, SESSION_MGR, TWILIO_MESSAGEQUEUE))
	api_handler.AddServlet("/kitchen", NewKitchenServlet(&server_config, SESSION_MGR))
	api_handler.AddServlet("/kitchenuser", NewKitchenUserServlet(&server_config, SESSION_MGR))
	api_handler.AddServlet("/mealrequest", NewMealRequestServlet(&server_config, SESSION_MGR, TWILIO_MESSAGEQUEUE))
	api_handler.AddServlet("/host", NewHostServlet(&server_config, SESSION_MGR, TWILIO_MESSAGEQUEUE))
	api_handler.AddServlet("/meal", NewMealServlet(&server_config, SESSION_MGR))
	api_handler.AddServlet("/review", NewReviewServlet(&server_config, SESSION_MGR))

	// Start listening to HTTP requests
	go http_server.ListenAndServe()
	log.Println("API Listening on " + bind_address)
}

func initTemplateServer() {
	bind_address := server_config.WWW.BindAddress + ":" + server_config.WWW.BindPort

	template_handler := NewTemplateHandler(
		&server_config,
		DB_CONN,
		TWILIO_MESSAGEQUEUE,
		SESSION_MGR,
	)

	http_server = http.Server{
		Addr:           bind_address,
		Handler:        template_handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	template_handler.HandleTemplate("request", template_request, "template_request.html")

	// Start listening to HTTP requests
	go http_server.ListenAndServe()

	log.Println("WWW Listening on " + bind_address)
}

func workTwilioQueue(client *twirest.TwilioClient, queue chan *SMS) {
	for {
		qmsg := <-queue

		msg := twirest.SendMessage{
			Text: qmsg.Message,
			To:   qmsg.To,
			From: server_config.Twilio.From}
		_, err := client.Request(msg)
		if err != nil {
			log.Println(err)
		}
	}
}

func main() {
	// Load CLI args
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting API server")

	/*
	 * Load Configuration
	 */
	server_config = LoadConfiguration(config_file)

	// Set config options that were loaded from CLI
	server_config.Arguments.LogToStderr = config_log_stderr

	/*
	 * Set up log facility
	 */
	if !config_log_stderr {
		if _, err := os.Stat(server_config.Logging.LogFile); os.IsNotExist(err) {
			log_file, err := os.Create(server_config.Logging.LogFile)
			if err != nil {
				log.Fatal("Log: Create: ", err.Error())
			}
			log.SetOutput(log_file)
		} else {
			log_file, err := os.OpenFile(server_config.Logging.LogFile, os.O_APPEND|os.O_RDWR, 0666)
			if err != nil {
				log.Fatal("Log: OpenFile: ", err.Error())
			}
			log.SetOutput(log_file)
		}
	}

	/*
	 * Set up signal handlers
	 */

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range c {
			log.Println("Exiting with signal", sig)
			os.Exit(1)
		}
	}()

	/*
	 * Start servers
	 */
	initHelpers()
	initApiServer()
	initTemplateServer()
	for {
		time.Sleep(time.Hour)
	}
}
