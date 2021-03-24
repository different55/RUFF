// Ruff provides a pop-up web server to Retrieve/Upload Files Fast over
// LAN, inspired by WOOF (Web Offer One File) by Simon Budig.
// 
// It's based on the principle that not everyone has <insert neat file transfer
// utility here>, just about every device that can network has an HTTP client,
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
	downloads int
	port int
	filePath string
	fileName string
	hideQR bool
	uploading bool
}

func getConfig() Config {
	conf := Config{
		downloads: 1,
		port: 8008,
		hideQR: false,
		uploading: false,
	}
	
	flag.IntVar(&conf.downloads, "count", conf.downloads, "number of downloads before exiting. set to -1 for unlimited downloads.")
	flag.IntVar(&conf.port, "port", conf.port, "port to serve file on.")
	flag.BoolVar(&conf.hideQR, "hide-qr", conf.hideQR, "hide the QR code.")
	flag.BoolVar(&conf.uploading, "upload", false, "upload files instead of downloading")
	
	flag.IntVar(&conf.downloads, "c", conf.downloads, "number of downloads before exiting. set to -1 for unlimited downloads. (shorthand)")
	flag.IntVar(&conf.port, "p", conf.port, "port to serve file on. (shorthand)")
	flag.BoolVar(&conf.hideQR, "q", conf.hideQR, "hide the QR code. (shorthand)")
	flag.BoolVar(&conf.uploading, "u", false, "upload files instead of downloading (shorthand)")

	flag.Parse()
	conf.filePath = flag.Arg(0)
	conf.fileName = path.Base(conf.filePath)

	return conf
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

func main() {
	conf := getConfig()

	server := &http.Server{
		Addr: fmt.Sprintf(":%v", conf.port),
		ReadTimeout: 10*time.Second,
		WriteTimeout: 10*time.Second,
	}

	if conf.uploading {
		setupUpload(server, &conf)
	} else {
		setupDownload(server, &conf)
	}

	ip, err := getIP()
	if err != nil {
		fmt.Println(err)
		return
	}

	url := fmt.Sprintf("http://%s:%v/%s", ip, conf.port, conf.fileName)
	if !conf.hideQR {
		qrterminal.GenerateHalfBlock(url, qrterminal.M, os.Stdout)
	}
	fmt.Println(url)

	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println(err)
		return
	}
}

func setupDownload(server *http.Server, conf *Config) {
	downloads := conf.downloads
	http.Handle("/", http.RedirectHandler("/"+conf.fileName, http.StatusFound)) // 302 redirect
	http.HandleFunc("/"+conf.fileName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+url.PathEscape(conf.fileName)+"\"")
		http.ServeFile(w, r, conf.filePath)
		
		downloads--
		if downloads == 0 {
			server.Shutdown(context.Background())
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

var uploadTemplate = `{{template "BaseHeader" "RUFF Upload Form"}}
		<form enctype="multipart/form-data" action="/" method="post">
			<label for="file">Select a file for upload:</label>
			<input type="file" name="file">
			<input type="submit" value="Upload"{{if .multiple}} multiple{{end}}>
		</form>
{{template "BaseFooter"}}`

var errorTemplate = `{{template "BaseHeader" "UploadError"}}
		<p>{{.}}</p>
		<p><a href="/">Go back</a></p>
{{template "BaseFooter"}}`

func setupUpload(server *http.Server, conf *Config) {
	tpl := template.Must(template.New("BaseHeader").Parse(baseHeader))
	template.Must(tpl.New("BaseFooter").Parse(baseFooter))
	template.Must(tpl.New("UploadForm").Parse(uploadTemplate))
	template.Must(tpl.New("UploadError").Parse(errorTemplate))
	
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
	})
}