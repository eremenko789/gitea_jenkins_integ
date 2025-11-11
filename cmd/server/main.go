package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gitea-jenkins-integ/internal/config"
	"github.com/gitea-jenkins-integ/internal/models"
	"github.com/gitea-jenkins-integ/internal/webhook"
)

func main() {
	// Загружаем конфигурацию
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Создаем обработчик вебхуков
	handler := webhook.NewHandler(cfg)

	// Настраиваем роутер
	router := gin.Default()

	// Эндпоинт для вебхуков от Gitea
	router.POST("/webhook", func(c *gin.Context) {
		var payload models.GiteaWebhookPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
			return
		}

		// Обрабатываем вебхук асинхронно
		if err := handler.HandleWebhook(&payload); err != nil {
			log.Printf("Error handling webhook: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process webhook"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "accepted"})
	})

	// Health check эндпоинт
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Запускаем сервер
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting server on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
