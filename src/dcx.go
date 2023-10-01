package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"os/user"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: dcx [options] [image-name]")
		fmt.Println("Default image (if none provided) is 'alpine'.")
		return
	}

	var removeContainer bool
	var imageName string

	// Check for 'rm' flag
	if os.Args[1] == "rm" {
		removeContainer = true
		if len(os.Args) >= 3 {
			imageName = os.Args[2]
		} else {
			imageName = "alpine" // Default to alpine if no image name is provided after 'rm'
		}
	} else {
		imageName = os.Args[1]
		if imageName == "" {
			imageName = "alpine"
		}
	}

	// Get the current directory
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error fetching current directory:", err)
		return
	}

	// Prepare volume binding argument
	volumeArg := fmt.Sprintf("%s:/shared", currentDir)

	// Step 1: Create a container
	dockerArgs := []string{"run", "-d", "-v", volumeArg}
	if removeContainer {
		dockerArgs = append(dockerArgs, "--rm")
	}
	dockerArgs = append(dockerArgs, imageName, "sleep", "infinity")
	containerCmd := exec.Command("docker", dockerArgs...)
	containerIDBytes, err := containerCmd.CombinedOutput()
	if err != nil {
		handleDockerError(err, containerIDBytes)
		return
	}

	containerID := strings.TrimSpace(string(containerIDBytes))

	// Check if bash exists in the container and install if necessary
	if !checkBashExists(containerID) {
		fmt.Println("Bash not found in the container. Installing...")
		if err := installBash(containerID); err != nil {
			fmt.Println(err)
			return
		}
	}

	currentUser, err := user.Current()
	if err != nil {
		fmt.Println("Error fetching current user:", err)
		return
	}

	hostUser := currentUser.Username
	hostPathEnv := fmt.Sprintf("HOST_PATH=%s", currentDir)
	ps1 := fmt.Sprintf("╭─\033[32m%s\033[0m in \033[33m$HOST_PATH\033[0m (host) and \033[34m\\w\033[0m (container)\n╰─○ ", hostUser)
	envPS1 := fmt.Sprintf("PS1=%s", ps1)

	echoCmd := exec.Command("docker", "exec", containerID, "bash", "-c", "echo 'export PS1=\"" + envPS1 + "\"' > /shared/env.sh")
	if err := echoCmd.Run(); err != nil {
		fmt.Println("Error setting up environment:", err)
		return
	}
	// Updated this line to set the working directory in the exec command
	attachCmd := exec.Command("docker", "exec", "-it", "-w", "/shared", "-e", hostPathEnv, "-e", envPS1, containerID, "bash")
	attachCmd.Stdin = os.Stdin
	attachCmd.Stdout = os.Stdout
	attachCmd.Stderr = os.Stderr
	if err := attachCmd.Run(); err != nil {
		fmt.Println("Error attaching to container:", err)
		return
	}

	if removeContainer {
		// Clean up container
		cleanCmd := exec.Command("docker", "rm", "-f", containerID)
		if err := cleanCmd.Run(); err != nil {
			fmt.Println("Error cleaning up container:", err)
		}
	}
}

func checkBashExists(containerID string) bool {
	// Try running a simple bash command in the container
	testCmd := exec.Command("docker", "exec", containerID, "bash", "-c", "echo 'bash exists'")
	if err := testCmd.Run(); err != nil {
		return false
	}
	return true
}


func installBash(containerID string) error {
	var bashInstallCmd *exec.Cmd

	// Probe the container to determine its base
	osReleaseCmd := exec.Command("docker", "exec", containerID, "cat", "/etc/os-release")
	osReleaseOutput, err := osReleaseCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("Error determining container base: %w", err)
	}

	osReleaseString := string(osReleaseOutput)
	if strings.Contains(osReleaseString, "ID=alpine") {
		bashInstallCmd = exec.Command("docker", "exec", containerID, "apk", "add", "--no-cache", "bash")
	} else if strings.Contains(osReleaseString, "ID=debian") || strings.Contains(osReleaseString, "ID=ubuntu") {
		// Assume it's debian-based and update package lists first
		updateCmd := exec.Command("docker", "exec", containerID, "apt-get", "update")
		if err := updateCmd.Run(); err != nil {
			return fmt.Errorf("Error updating package lists: %w", err)
		}
		bashInstallCmd = exec.Command("docker", "exec", containerID, "apt-get", "install", "-y", "bash")
	} else if strings.Contains(osReleaseString, "ID=centos") || strings.Contains(osReleaseString, "ID=fedora") {
		// For CentOS or Fedora, we'd use `yum` or `dnf`. But for simplicity, let's just handle CentOS with `yum`.
		bashInstallCmd = exec.Command("docker", "exec", containerID, "yum", "install", "-y", "bash")
	} else {
		return fmt.Errorf("Unsupported container base detected")
	}

	if err := bashInstallCmd.Run(); err != nil {
		return fmt.Errorf("Error installing Bash: %w", err)
	}

	return nil
}

func handleDockerError(err error, output []byte) {
	fmt.Println("Error creating container:", err)
	fmt.Println("Docker output:", string(output))

	// Verbose logging for common Docker errors
	if exitError, ok := err.(*exec.ExitError); ok {
		switch exitError.ExitCode() {
		case 125:
			fmt.Println("Is the Docker daemon running?")
		case 127:
			fmt.Println("Command not found inside the container.")
		default:
			fmt.Printf("Unhandled Docker error with exit code: %d\n", exitError.ExitCode())
		}
	}
}
