package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nadoo/glider/pkg/log"
	"github.com/nadoo/glider/rule"
	"github.com/nadoo/glider/stats"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	addr   string
	proxy  *rule.Proxy
	static fs.FS
}

func New(addr string, proxy *rule.Proxy) *Server {
	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	return &Server{addr: addr, proxy: proxy, static: static}
}

func (s *Server) ListenAndServe() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/status")
	})
	router.GET("/shared.css", s.serveAsset("shared.css", "text/css; charset=utf-8"))
	router.GET("/status", s.servePage("status.html"))
	router.GET("/lha", s.servePage("lha.html"))
	router.GET("/logs", s.servePage("logs.html"))
	router.GET("/traffic", s.servePage("traffic.html"))
	router.GET("/api/status", s.handleStatus)
	router.GET("/api/lha/history", s.handleLHAHistory)
	router.GET("/api/logs", s.handleLogs)
	router.GET("/api/traffic", s.handleTraffic)

	log.Printf("[web] listening on %s", s.addr)
	if err := router.Run(s.addr); err != nil {
		log.Fatalf("[web] failed to listen on %s: %v", s.addr, err)
	}
}

func (s *Server) handleLHAHistory(c *gin.Context) {
	date := time.Now()
	if raw := c.Query("date"); raw != "" {
		parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "date must use YYYY-MM-DD"})
			return
		}
		date = parsed
	}
	c.JSON(http.StatusOK, stats.AvailabilityForDay(date))
}

func (s *Server) servePage(name string) gin.HandlerFunc {
	return s.serveAsset(name, "text/html; charset=utf-8")
}

func (s *Server) serveAsset(name, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		content, err := fs.ReadFile(s.static, name)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed to load page")
			return
		}

		c.Data(http.StatusOK, contentType, content)
	}
}

func (s *Server) handleStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"groups": s.proxy.StatusSnapshot(),
	})
}

func (s *Server) handleLogs(c *gin.Context) {
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil && value > 0 {
			limit = value
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	logs := log.RequestRecent(limit)
	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
		"limit": limit,
	})
}

func (s *Server) handleTraffic(c *gin.Context) {
	records := stats.Snapshot()

	var uploadTotal uint64
	var downloadTotal uint64
	for _, record := range records {
		uploadTotal += record.UploadBytes
		downloadTotal += record.DownloadBytes
	}

	c.JSON(http.StatusOK, gin.H{
		"records": records,
		"summary": gin.H{
			"source_count":   len(records),
			"upload_bytes":   uploadTotal,
			"download_bytes": downloadTotal,
			"total_bytes":    uploadTotal + downloadTotal,
		},
	})
}
