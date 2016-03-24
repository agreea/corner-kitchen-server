package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"code.google.com/p/go-uuid/uuid"
	"io/ioutil"
	"os"
	"sort"
	// "time"
	"bytes"
	"path/filepath"
	"image/gif"
	"image"
	"syscall"
	"os/exec"
	"github.com/disintegration/imaging"
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
	// read_timeout, err := time.ParseDuration(server_config.Network.ReadTimeout)
	// write_timeout, err := time.ParseDuration(server_config.Network.WriteTimeout)
	// http_server = http.Server{
	// 	Addr:           "0.0.0.0:1337",
	// 	Handler:        t,
	// 	ReadTimeout:    read_timeout * 3,
	// 	WriteTimeout:   write_timeout * 3,
	// 	MaxHeaderBytes: 1 << 20,
	// }
	return t
}

func (t *GifServlet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get video from req
	log.Println("Serving http")
	apiResult := t.Upload(r)
	ServeData(w, r, apiResult)
}

// func (t *GifServlet) upload_listen(){
//   // listen on all interfaces
//   ln, _ := net.Listen("tcp", "127.0.0.1:1337")

//   // accept connection on port
//   conn, _ := ln.Accept()
//   defer ln.Close()
//   // run loop forever (or until ctrl-c)
//   for {
//     go handleRequest(conn)
//   }

// }

// func handleRequest(conn net.Conn) {
//   // Make a buffer to hold incoming data.
//   buf := make([]byte, 15000000)
//   log.Println("handling request")
//   // en
//   // Read the incoming connection into the buffer.
//   _, err := conn.Read(buf)
//   if err != nil {
//     log.Println(err)
//   }
//   filename, err := CreateGifFile(buf)
//   if err != nil {
//     log.Println(err)
//   }
//   // Send a response back to person contacting us.
//   conn.Write([]byte(filename))
//   // Close the connection when you're done with it.
//   conn.Close()
// }

func (t *GifServlet) Upload(r *http.Request) *ApiResult {
	// get video from req
	log.Println("in upload")
 	buf, err := ioutil.ReadAll(r.Body) //<--- here!
 	length := len(buf)
 	log.Println(length)
 	if err != nil {
 		log.Println(err)
 		return APIError("Failed to read video", 400)
 	}
	defer r.Body.Close()
	// write to file -- filename does NOT include .mov/.gif/.jpeg ending
	file_name, err := t.write_video(buf)
	if err != nil {
		log.Println(err)
		return APIError("Could not encode video", 400)
	}
	log.Println(file_name)
	// using command line mmpeg, convert to 15fps, downsample it
	if err = t.downsample_video(file_name); err != nil {
		log.Println(err)
		return APIError("Could not downsample", 500)
	}
	if err = t.extract_frames(file_name); err != nil {
		log.Println(err)
		return APIError("Could not extract frames", 500)
	}
	if err := t.encode_gif(file_name); err != nil {
		log.Println(err)
		return APIError("Could not encode gif", 400)	
	}
	return APISuccess("OK")
}

func (t *GifServlet) write_video(buf []byte) (string, error) {
	// generate the file name and address
	// TODO: retrieve file type from the header
	file_name := uuid.New()
	absolute_file_address := t.server_config.Image.Path + file_name + ".MOV"
	log.Println(file_name)
	syscall.Umask(022)
	err := ioutil.WriteFile(absolute_file_address, buf, os.FileMode(int(0664)))
	if err != nil {
		return "", err
	} else {
		file, err := os.Open(absolute_file_address)
     	if err != nil {
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

// WORKS!
func (t *GifServlet) downsample_video(file_name string) error {
	// use command line to execute -r 15, downsampling
	cmd := "ffmpeg"
	root_dir := t.server_config.Image.Path
	input_file_path := root_dir + file_name + ".MOV"
	output_file_path := root_dir + "ds_" + file_name + ".MOV"
	args := []string{"-i", input_file_path, "-r", "10", "-vf", "scale=iw/2:-1", output_file_path, "-c:v", "libx264"}
	if err := exec.Command(cmd, args...).Run(); err != nil {
		log.Println(err)
		return err
	}
	// TODO: delete original once finished here
	return nil
}

//
func (t *GifServlet) extract_frames(file_name string) error {
	// make a directory titled directory_name
	root_dir := t.server_config.Image.Path
	input_file_path := root_dir + "ds_" + file_name + ".MOV"
	output_dir := root_dir + file_name + "/"
	syscall.Umask(022)
	if err := os.Mkdir(output_dir, os.FileMode(int(0775))); err != nil {
		log.Println(err)
		return err
	}
	cmd := "ffmpeg"
	args := []string{"-i", input_file_path, output_dir + "%03d.bmp"}
	if cmdOut, err := exec.Command(cmd, args...).Output(); err != nil {
		log.Println(cmdOut)
		log.Println(err)
		return err
	}
	// TODO: delete downsample once finished here
	return nil
}

func (t *GifServlet) encode_gif(file_name string) error {
	gif_directory := t.server_config.Image.Path + file_name + "/"
    gif_struct := t.get_gif_from_dir(gif_directory)
	// encode them to GIF file
	gif_file, err := os.Create(gif_directory + "loopy.gif")
	if err != nil {
		log.Println(err)
		return err
	}
	if err := gif.EncodeAll(gif_file, &gif_struct); err != nil {
		log.Println(err)
		return err
	}
	gif_file.Close()
	// (eventually, delete all files that are not 0.jpg)
	return nil
}

func (t *GifServlet) get_gif_from_dir(directory_name string) gif.GIF {
	outGif := gif.GIF{}
	file_pattern :=  directory_name + "*.bmp"
    filenames, _ := filepath.Glob(file_pattern)
    sort.Strings(filenames)
    for _, filename := range filenames {
    	log.Println(filename)
		// encode them from bmp to GIF, add to frames
		img, err := imaging.Open(filename)
		if err != nil {
			log.Println("Skipping file %s due to error reading it :%s", filename, err)
			continue
		}
		buf := bytes.Buffer{}
		if err := gif.Encode(&buf, img, nil); err != nil {
			log.Println("Skipping file %s due to error in gif encoding:%s", filename, err)
			continue
		}
		frame, err := gif.Decode(&buf)
		if err != nil {
			log.Printf("Skipping file %s due to weird error reading the temporary gif :%s", filename, err)
			continue
		}
		outGif.Image = append(outGif.Image, frame.(*image.Paletted))
		outGif.Delay = append(outGif.Delay, 0)
    }
	return outGif
}