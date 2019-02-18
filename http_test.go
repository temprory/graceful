package graceful

import (
	"github.com/didip/tollbooth"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/temprory/log"
	"io"
	"net/http"
	"os"
	"testing"
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
func TestHttpServer(t *testing.T) {
	initLog()

	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.GET("/hello", func(c *gin.Context) {
		logInfo("onHello")
		c.String(http.StatusOK, "hello")
	})

	addr := ":8080"
	Serve(addr, tollbooth.LimitFuncHandler(tollbooth.NewLimiter(100, nil), router.ServeHTTP), time.Second*5, nil, func() {
		os.Exit(0)
	})
}
