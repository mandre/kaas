package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jasonlvhit/gocron"

	"github.com/vrutkovs/kaas/pkg/kaas"
)

// health is k8s endpoint for liveness check
func health(c *gin.Context) {
	c.String(http.StatusOK, "")
}

func main() {
	kubeConfigEnvVar := os.Getenv("KUBECONFIG")

	k8sC, routeC, err := kaas.TryLogin(kubeConfigEnvVar)
	if err != nil {
		log.Println("Failed to login in cluster")
		log.Println(err)
		return
	}

	namespace := "kaas"
	envVarNamespace := os.Getenv("NAMESPACE")
	if len(envVarNamespace) != 0 {
		namespace = envVarNamespace
	}

	rquotaName := "pod-quota"
	envVarRquotaName := os.Getenv("QUOTA_NAME")
	if len(envVarRquotaName) != 0 {
		rquotaName = envVarRquotaName
	}

	rqStatus := kaas.RQuotaStatus{}

	server := &kaas.ServerSettings{
		K8sClient:   k8sC,
		RouteClient: routeC,
		Namespace:   namespace,
		RQuotaName:  rquotaName,
		RQStatus:    &rqStatus,
		Conns:       make(map[string]*websocket.Conn),
		Datasources: make(map[string]int),
	}
	if server.GetResourceQuota() != nil {
		fmt.Print("Failed to read initial resource quota")
	} else {
		go server.WatchResourceQuota()
	}

	r := gin.New()
	r.SetTrustedProxies(nil)

	// Server static HTML
	r.Use(static.Serve("/", static.LocalFile("./html", true)))

	// Don't log k8s health endpoint
	r.Use(
		gin.LoggerWithWriter(gin.DefaultWriter, "/health"),
		gin.Recovery(),
	)
	r.GET("/health", health)
	r.GET("/ws/status", server.HandleStatusViaWS)

	go func() {
		gocron.Every(2).Minutes().Do(server.CleanupOldDeployements)
		<-gocron.Start()
	}()

	r.Run(":8080")
}
