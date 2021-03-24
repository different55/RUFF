// Ruff provides a pop-up web server to Retrieve/Upload Files Fast over
// LAN, inspired by WOOF (Web Offer One File) by Simon Budig.
// 
// It's based on the idea that not every device has <insert neat file transfer
// tool here>, but just about every device that can network has an HTTP client,
// making a hyper-simple HTTP server a viable option for file transfer with
// zero notice or setup as long as *somebody* has a copy of RUFF.
//
// Why create RUFF when WOOF exists? WOOF is no longer in the debian repos and
// it's easier to `go get` a tool than it is to hunt down Simon's website for
// the latest copy.
package main

import (
	"net"
	"net/url"
	"net/http"
	"time"
	"context"
	"html/template"
	
	"path"
	"os"
	"io"

	"fmt"
	"flag"
	"errors"
	"github.com/mdp/qrterminal"
)

// Config stores all settings for an instance of RUFF.
type Config struct {
	Downloads int
	Port int
	FilePath string
	FileName string
	HideQR bool
	Uploading bool
	Multiple bool
}

func getConfig() (Config, error) {
	conf := Config{
		Downloads: 1,
		Port: 8008,
		HideQR: false,
		Uploading: false,
		Multiple: true,
	}
	
	flag.IntVar(&conf.Downloads, "count", conf.Downloads, "number of downloads before exiting. set to -1 for unlimited downloads.")
	flag.IntVar(&conf.Port, "port", conf.Port, "port to serve file on.")
	flag.BoolVar(&conf.HideQR, "hide-qr", conf.HideQR, "hide the QR code.")
	flag.BoolVar(&conf.Uploading, "upload", false, "upload files instead of downloading")
	flag.BoolVar(&conf.Multiple, "multiple", conf.Multiple, "allow uploading multiple files at once")
	
	flag.IntVar(&conf.Downloads, "c", conf.Downloads, "number of downloads before exiting. set to -1 for unlimited downloads. (shorthand)")
	flag.IntVar(&conf.Port, "p", conf.Port, "port to serve file on. (shorthand)")
	flag.BoolVar(&conf.HideQR, "q", conf.HideQR, "hide the QR code. (shorthand)")
	flag.BoolVar(&conf.Uploading, "u", false, "upload files instead of downloading (shorthand)")
	flag.BoolVar(&conf.Multiple, "m", conf.Multiple, "allow uploading multiple files at once (shorthand)")

	flag.Parse()
	conf.FilePath = flag.Arg(0)
	conf.FileName = path.Base(conf.FilePath)

	if conf.FilePath == "" && !conf.Uploading {
		return conf, errors.New("no file provided to download")
	}

	return conf, nil
}

func getIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "", err
	}
	
	return localAddr.IP.String(), nil
}

var done = make(chan struct{})

func main() {
	conf, err := getConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr: fmt.Sprintf(":%v", conf.Port),
		ReadTimeout: 10*time.Second,
		WriteTimeout: 10*time.Second,
	}

	if conf.Uploading {
		setupUpload(server, conf)
	} else {
		setupDownload(server, conf)
	}

	ip, err := getIP()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://%s:%v/%s", ip, conf.Port, conf.FileName)
	if !conf.HideQR {
		qrterminal.GenerateHalfBlock(url, qrterminal.M, os.Stdout)
	}
	fmt.Println(url)

	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
		os.Exit(1)
	}
	
	// Wait for the server to finish any transfers, up to 3 seconds
	select {
		case <-done:
		case <-time.After(3*time.Second):
	}
}

func setupDownload(server *http.Server, conf Config) {
	downloads := conf.Downloads
	http.Handle("/", http.RedirectHandler("/"+conf.FileName, http.StatusFound)) // 302 redirect
	http.HandleFunc("/"+conf.FileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+url.PathEscape(conf.FileName)+"\"")
		http.ServeFile(w, r, conf.FilePath)
		
		downloads--
		if downloads == 0 {
			go shutdown(server)
		}
	})
}

var baseHeader = `<!DOCTYPE html>
<html>
	<head>
		<title>{{.}}</title>
		<style>
			body {
				margin: 18pt auto;
				font-size: 16pt;
				color: #212121;
			}
		</style>
	</head>
	<body>`

var baseFooter = `</body>
</html>`

var uploadTemplate = `{{template "BaseHeader" "RUFF - Upload Form"}}
		<form enctype="multipart/form-data" action="/" method="post">
			<label for="file">Select a file for upload:</label>
			<input type="file" name="file">
			<input type="submit" value="Upload"{{if .Multiple}} multiple{{end}}>
		</form>
{{template "BaseFooter"}}`

var errorTemplate = `{{template "BaseHeader" "RUFF - Upload Error"}}
		<p>{{.}}</p>
		<p><a href="/">Go back</a></p>
{{template "BaseFooter"}}`

var messageTemplate = `{{template "BaseHeader" (print "RUFF - " .)}}
		<p>{{.}}</p>
{{template "BaseFooter"}}`

func setupUpload(server *http.Server, conf Config) {
	tpl := template.Must(template.New("BaseHeader").Parse(baseHeader))
	template.Must(tpl.New("BaseFooter").Parse(baseFooter))
	template.Must(tpl.New("UploadForm").Parse(uploadTemplate))
	template.Must(tpl.New("UploadError").Parse(errorTemplate))
	template.Must(tpl.New("UploadMessage").Parse(messageTemplate))
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Upload form
		if r.Method != http.MethodPost {
			err := tpl.ExecuteTemplate(w, "UploadForm", conf)
			if err != nil {
				panic(err)
			}
			return
		}

		// Handle upload
		r.ParseMultipartForm(20 << 20) // Buffer a maximum of 20MB in memory.
		
		inFile, header, err := r.FormFile("file")
		if err != nil {
			tpl.ExecuteTemplate(w, "UploadError", err)
			return
		}
		defer inFile.Close()

		outFile, err := os.Create(header.Filename)
		if err != nil {
			tpl.ExecuteTemplate(w, "UploadError", err)
			return
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, inFile)
		if err != nil {
			tpl.ExecuteTemplate(w, "UploadError", err)
			return
		}

		tpl.ExecuteTemplate(w, "UploadMessage", "Upload successful!")
		go shutdown(server)
	})
}

func shutdown(server *http.Server) {
	server.Shutdown(context.Background())
	done <- struct{}{}
}