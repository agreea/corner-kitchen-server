package main

import (
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

func init_server() {

	bind_address := server_config.Network.BindAddress + ":" + server_config.Network.BindPort

	api_handler := NewApiHandler(&server_config)

	read_timeout, err := time.ParseDuration(server_config.Network.ReadTimeout)
	if err != nil {
		read_timeout = 10 * time.Second
		log.Println("Invalid network timeout", server_config.Network.ReadTimeout)
	}

	write_timeout, err := time.ParseDuration(server_config.Network.WriteTimeout)
	if err != nil {
		write_timeout = 10 * time.Second
		log.Println("Invalid network timeout", server_config.Network.WriteTimeout)
	}

	http_server = http.Server{
		Addr:           bind_address,
		Handler:        api_handler,
		ReadTimeout:    read_timeout,
		WriteTimeout:   write_timeout,
		MaxHeaderBytes: 1 << 20,
	}

	session_manager := NewSessionManager(&server_config)
	api_handler.AddServlet("/version", NewVersionServlet())
	api_handler.AddServlet("/user", NewUserServlet(&server_config, session_manager))
	api_handler.AddServlet("/truck", NewTruckServlet(&server_config))

	// Start listening to HTTP requests
	if err := http_server.ListenAndServe(); err != nil {
		log.Fatalln("Fatal Error: ListenAndServe: ", err.Error())
	}

	log.Println("Listening on " + bind_address)
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
	 * Start server
	 */
	init_server()
}
