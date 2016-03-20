package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net"
	"net/http"
	"code.google.com/p/go-uuid/uuid"
	"io/ioutil"
	// "strings"
	// "bufio"
	"encoding/base64"
	"syscall"
	"os"
)

type GifServlet struct {
	db              *sql.DB
	server_config   *Config
}

func NewGifServlet(server_config *Config) *GifServlet {
	t := new(GifServlet)

	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("New Gif servlet", "Failed to open database:", err)
	}
	t.db = db
	go t.upload_listen()
	return t
}

func (t *GifServlet) upload_listen(){
  // listen on all interfaces
  ln, _ := net.Listen("tcp", "127.0.0.1:1337")

  // accept connection on port
  conn, _ := ln.Accept()
  defer ln.Close()
  // run loop forever (or until ctrl-c)
  for {
    go handleRequest(conn)
  }

}

func handleRequest(conn net.Conn) {
  // Make a buffer to hold incoming data.
  buf := make([]byte, 1024)
  // en
  // Read the incoming connection into the buffer.
  _, err := conn.Read(buf)
  if err != nil {
    log.Println(err)
  }
  filename, err := CreateGifFile(buf)
  if err != nil {
    log.Println(err)
  }
  // Send a response back to person contacting us.
  conn.Write(filename)
  // Close the connection when you're done with it.
  conn.Close()
}

func (t *GifServlet) Upload(r *http.Request) *ApiResult {
	// get gif
	// write to file
	// get your permissions right
	// return the URL
	gif_str := r.Form.Get("gif")
	log.Println(gif_str)
	file_name, err := CreateGifFile(gif_str)
	if err != nil {
		log.Println(err)
		return APIError("Failed to create GIF", 500)
	}
	return APISuccess("https://yaychakula.com/img/" + file_name)
}

func CreateGifFile(data []byte) (string, error) {
	// generate the file name and address
	file_name := uuid.New() + ".gif"
	file_address := "/var/www/prod/img/" + file_name
	log.Println(file_name)
	syscall.Umask(022)
	err = ioutil.WriteFile(file_address, data, os.FileMode(int(0664)))
	if err != nil {
		return "", err
	} else {
		file, err := os.Open(file_address)
     	if err != nil {
         // handle the error here
         return "", err
     	}
     	defer file.Close()
	   stat, err := file.Stat()
	   if err != nil {
	       return "", err
	   }
	   log.Println(stat)
	}
	return file_name, nil
}