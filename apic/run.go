package apic

import (
	"caddydash/config"
	"context"
	"fmt"
	"net/http"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/infinite-iroha/touka"
)

type CaddyRunning struct {
	running bool
	mu      sync.Mutex
}

func (c *CaddyRunning) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *CaddyRunning) SetRunning(running bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = running
}

var caddyRunning = &CaddyRunning{}

func RunCaddy(cfg *config.Config) error {
	if caddyRunning.IsRunning() {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	log.Printf("Starting Caddy in directory: %s", cfg.Server.CaddyDir)
	cmd := exec.CommandContext(ctx, "./caddy", "run", "--config", "Caddyfile")
	cmd.Dir = cfg.Server.CaddyDir
	logFile, err := os.OpenFile(filepath.Join(cfg.Server.CaddyDir, "log", "caddystdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		cancel()
		log.Printf("Failed to open log file: %v", err)
		return err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	caddyRunning.SetRunning(true)

	err = cmd.Start()
	if err != nil {
		cancel()
		caddyRunning.SetRunning(false)
		log.Printf("Failed to start Caddy process: %v", err)
		return err
	}

	go func() {
		waitErr := cmd.Wait()
		caddyRunning.SetRunning(false)
		log.Printf("Caddy process exited with error: %v", waitErr)
		cancel()
		if waitErr != nil {
			// 如果 Caddy 非正常退出（例如崩溃），Wait() 会返回 *exec.ExitError
			// 对于优雅关闭，如果 Caddy 接收到 SIGTERM 并正常退出，Wait() 返回 nil
			// 只有当进程以非零状态码退出时，Wait() 才返回非 nil 的 ExitError
			// 这里的日志输出应由外部日志库处理，而不是标准库log
			if exitErr, ok := waitErr.(*exec.ExitError); ok {
				log.Printf("Caddy process exited with non-zero status: %v", exitErr)
			} else {
				log.Printf("Caddy process exited with error: %v", waitErr)
			}
		} else {
			log.Println("Caddy process exited gracefully.")
		}
	}()
	return nil
}

func stopCaddyProcess() error {
	if !caddyRunning.IsRunning() {
		return nil
	}

	resp, err := http.Post("http://localhost:2019/stop", "text/plain", strings.NewReader(""))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop Caddy: status %d", resp.StatusCode)
	}

	return nil
}

// RestartCaddyProcess stops and starts Caddy synchronously.
func RestartCaddyProcess(cfg *config.Config) error {
	if !caddyRunning.IsRunning() {
		return RunCaddy(cfg)
	}

	if err := stopCaddyProcess(); err != nil {
		return err
	}

	for caddyRunning.IsRunning() {
		time.Sleep(500 * time.Millisecond)
	}

	return RunCaddy(cfg)
}

func StartCaddy(cfg *config.Config) touka.HandlerFunc {
	return func(c *touka.Context) {
		if caddyRunning.IsRunning() {
			c.JSON(200, map[string]string{"message": "Caddy is already running"})
			return
		}
		go func() {
			if err := RunCaddy(cfg); err != nil {
				c.Errorf("Failed to start Caddy: %v", err)
				c.JSON(500, map[string]string{"error": err.Error()})
				return
			}
		}()
		c.JSON(200, map[string]string{"message": "Caddy is starting"})
	}
}

func IsCaddyRunning() touka.HandlerFunc {
	return func(c *touka.Context) {
		if caddyRunning.IsRunning() {
			c.JSON(200, map[string]string{"message": "Caddy is running"})
		} else {
			c.JSON(200, map[string]string{"message": "Caddy is not running"})
		}
	}
}

func StopCaddy() touka.HandlerFunc {
	return func(c *touka.Context) {
		if !caddyRunning.IsRunning() {
			c.JSON(200, map[string]string{"message": "Caddy is not running"})
			return
		}
		if err := stopCaddyProcess(); err != nil {
			c.JSON(500, map[string]string{"error": err.Error()})
			return
		}

		c.JSON(200, map[string]string{"message": "Caddy stopped successfully"})
	}
}

func RestartCaddy(cfg *config.Config) touka.HandlerFunc {
	return func(c *touka.Context) {
		if err := RestartCaddyProcess(cfg); err != nil {
			c.JSON(500, map[string]string{"error": err.Error()})
			return
		}
		c.JSON(200, map[string]string{"message": "Caddy is restarting"})

	}
}
