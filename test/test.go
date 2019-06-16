package main

import (
	"github.com/didip/tollbooth"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/temprory/graceful"
	"github.com/temprory/log"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

func initLog() {
	logPrefix := "[APP] "
	timeLayout := "2006/01/02"
	fileWriter := &log.FileWriter{
		RootDir:     "./logs/",
		DirFormat:   "",
		FileFormat:  "20060102.log",
		TimeBegin:   len(logPrefix),
		TimePrefix:  timeLayout,
		MaxFileSize: 1024 * 1024 * 32,
		EnableBufio: false,
	}
	out := io.MultiWriter(os.Stdout, fileWriter)

	layout := logPrefix + timeLayout
	log.SetOutput(out)
	log.SetLogTimeFormat(layout)
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()
	gin.DefaultWriter = out
}

//test url: http://localhost:8080/hello
func main() {
	initLog()

	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Any("/hello", func(c *gin.Context) {
		log.Info("onHello")
		body, err := ioutil.ReadAll(c.Request.Body)
		log.Println("body:", err, string(body))
		c.String(http.StatusOK, "hello")
	})

	addr := ":8080"
	svr, err := graceful.NewHttpServer(addr, tollbooth.LimitFuncHandler(tollbooth.NewLimiter(100, nil), router.ServeHTTP), time.Second*5, nil, func() {
		os.Exit(-1)
	})

	//http://localhost:8080/debug/pprof/profile
	//go tool pprof -http=:8080 profile
	svr.EnablePProf("/debug/pprof/")

	if err != nil {
		log.Fatal("new server failed: %v", err)
	}

	svr.Serve()
}
