# Go-Toralizer

Go-Toralizer is a CLI tool that allows you to execute commands through the Tor network. It ensures that your commands are routed through the Tor proxy, providing anonymity and privacy.

## Features

- Check if the Tor proxy is running.
- Verify the IP address to ensure the connection is routed through the Tor network.
- Execute specified commands through the Tor proxy.

## Prerequisites

- Go (Golang) installed on your machine.
- Tor installed and running on your machine.

## Installation

1. **Clone the repository**:
   ```sh
   git clone https://github.com/zvdy/go-toralizer.git
   cd go-toralizer
   ```

2. **Build the application**:
   ```sh
   go build -o go-toralizer
   ```

## Usage

1. **Ensure Tor is running**:
   ```sh
   tor
   ```

2. **Execute a command through Tor**:
   ```sh
   ./go-toralizer exec --command "curl http://httpbin.org/ip"
   ```

   This command will route the `curl` request through the Tor network and display the IP address from which the request is made.

## Example Output

When you run the command, you should see output similar to the following:

```sh
Tor proxy is running
Tor verification output: {"IsTor":true,"IP":"185.220.101.1"}
{
  "origin": "185.220.101.1"
}
```

## Setup

1. **Build the application**:
   ```sh
   go build -o go-toralizer
   ```

2. **Move the binary to a directory in your PATH** (optional but recommended for easier access):
   ```sh
   sudo mv go-toralizer /usr/local/bin/
   ```

3. **Create an alias** (optional, for convenience):
   Add the following line to your shell configuration file (`~/.bashrc`, `~/.zshrc`, etc.):
   ```sh
   alias toralizer='/usr/local/bin/go-toralizer exec --command'
   ```

   Then, reload your shell configuration:
   ```sh
   source ~/.bashrc  # or source ~/.zshrc
   ```

4. **Use the tool**:
   You can now use the `go-toralizer` to run any command through the Tor network. Here are some examples:

   - **Without alias**:
     ```sh
     ./go-toralizer exec --command "ping -c 4 website.com"
     ./go-toralizer exec --command "curl http://website.com"
     ./go-toralizer exec --command "wget http://website.com"
     ```

   - **With alias**:
     ```sh
     toralizer "ping -c 4 website.com"
     toralizer "curl http://website.com"
     toralizer "wget http://website.com"
     ```

### Example Usage

- **Ping a website**:
  ```sh
  toralizer "ping -c 4 website.com"
  ```

- **Curl a website**:
  ```sh
  toralizer "curl http://website.com"
  ```

- **Download a file using wget**:
  ```sh
  toralizer "wget http://website.com/file.zip"
  ```

This setup ensures that any command you run through `toralizer` will use the Tor network for its TCP connections. Make sure Tor is running on your machine and listening on port 9050. You can start Tor with the command `tor` if it's installed.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

