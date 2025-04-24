package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

const pollInterval = 5 * time.Minute

var (
	apiURL      string
	apiKey      string
	status      string
	activeAt    time.Time
	statusMutex sync.RWMutex
)

type ApiResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Status        string    `json:"status"`
		ConnsActiveAt time.Time `json:"conns_active_at"`
	} `json:"result"`
}

func loadEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	accountID := os.Getenv("ACCOUNT_ID")
	tunnelID := os.Getenv("TUNNEL_ID")
	apiKey = os.Getenv("API_TOKEN")
	if accountID == "" || tunnelID == "" || apiKey == "" {
		log.Fatal("ACCOUNT_ID, TUNNEL_ID, and API_TOKEN must be set in the environment variables")
	}

	apiURL = fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel/%s", accountID, tunnelID)
}

func pollAPI() {
	for {
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			log.Printf("Error creating request: %v", err)
			time.Sleep(pollInterval)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error polling API: %v", err)
			time.Sleep(pollInterval)
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Error reading API response: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		var apiResponse ApiResponse
		if err := json.Unmarshal(body, &apiResponse); err != nil {
			log.Printf("Error parsing API response: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		if apiResponse.Success {
			statusMutex.Lock()
			status = apiResponse.Result.Status
			activeAt = apiResponse.Result.ConnsActiveAt
			statusMutex.Unlock()
		} else {
			log.Printf("API response indicates failure: %s", string(body))
		}

		time.Sleep(pollInterval)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	statusMutex.RLock()
	defer statusMutex.RUnlock()

	uptime := time.Since(activeAt).Truncate(time.Second)

	var statusColor string
	switch status {
	case "healthy":
		statusColor = "green"
	case "inactive":
		statusColor = "darkslategray"
	case "degraded":
		statusColor = "orangered"
	case "down":
		statusColor = "red"
	default:
		statusColor = "darkslategray"
	}

	response := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<title>Server Status</title>
	<style>
			body {
					font-family: Arial, sans-serif;
					text-align: center;
					display: flex;
					flex-direction: column;
					justify-content: center;
					align-items: center;
					height: 100dvh;
					height: 100vh;
					margin: 0;
					background-color: #121212;
					color: white;
			}
			.status-pill {
					display: inline-block;
					padding: 10px 20px;
					color: white;
					background-color: %s;
					border-radius: 25px;
					font-size: 1.2em;
					text-transform: uppercase;
			}
	</style>
	<script>
		let uptimeSeconds = %d;

		function updateUptime() {
			uptimeSeconds++;
			const uptimeElement = document.getElementById("uptime");
			const hours = Math.floor(uptimeSeconds / 3600);
			const minutes = Math.floor((uptimeSeconds %% 3600) / 60);
			const seconds = uptimeSeconds %% 60;
			uptimeElement.textContent = hours + "h" + minutes + "m" + seconds + "s";
		}

		function refreshPage() { location.reload(); };

		setInterval(updateUptime, 1000);
		setTimeout(refreshPage, 300000);
	</script>
</head>
<body>
	<h1>Server Status</h1>
	<div class="status-pill">%s</div>
	<p>Uptime: <span id="uptime">%s</span></p>
</body>
</html>`, statusColor, int(uptime.Seconds()), status, uptime.String())

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

func main() {
	loadEnv()

	go pollAPI()

	http.HandleFunc("/", handler)
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server started on :" + port)
	log.Println("Polling API every", pollInterval)
	log.Println("Press Ctrl+C to stop the server")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
