# go-toralizer
<img src="https://adrianalonso.es/wp-content/uploads/2018/06/go-lenguaje-programacion.png" alt="Tor Logo" width="200" height="100"/><img src="https://upload.wikimedia.org/wikipedia/commons/thumb/1/15/Tor-logo-2011-flat.svg/1200px-Tor-logo-2011-flat.svg.png" alt="Tor Logo" width="200" height="100"/>

---

`go-toralizer` is a CLI tool to run commands through the Tor network. It ensures your commands are executed with the anonymity provided by Tor.

## Installation

1. **Clone the repository:**
    ```sh
    git clone https://github.com/zvdy/go-toralizer.git
    cd go-toralizer
    ```

2. **Build the project:**
    ```sh
    go build -o go-toralizer main.go
    ```

3. **Move the binary to a directory in your PATH:**
    ```sh
    sudo mv go-toralizer /usr/local/bin/
    ```

## Alias Creation

To simplify the usage of `go-toralizer`, you can create an alias in your shell configuration file (e.g., `.bashrc`, `.zshrc`).

1. **Open your shell configuration file:**
    ```sh
    nano ~/.bashrc  # or ~/.zshrc for zsh users
    ```

2. **Add the following line to create an alias:**
    ```sh
    alias toralizer='go-toralizer'
    ```
      
      - You can also add the following alias if you want default cli and simpler `cmd` concatenation 
         ```sh
         toralizer() {
            /usr/local/bin/go-toralizer exec --command "$*"
         }

         # And then you will be able to:
         toralzier ping website.com
         ```

3. **Reload your shell configuration:**
    ```sh
    source ~/.bashrc  # or ~/.zshrc for zsh users
    ```

## Usage

### Basic Command Execution

To execute a command through Tor, use the `exec` command with the `--command` flag:

```sh
toralizer exec --command "curl http://example.com"
```

### Verbose Output

Enable verbose output to see more details about the execution:

```sh
toralizer exec --command "curl http://example.com" --verbose
```

### Specify Timeout

Set a custom timeout for the command execution:

```sh
toralizer exec --command "curl http://example.com" --timeout 60s
```

### Custom Proxy Address

Use a custom proxy address instead of the default `socks5://127.0.0.1:9050`:

```sh
toralizer exec --command "curl http://example.com" --proxy "socks5://127.0.0.1:9150"
```

### Redirect Output to a File

Redirect the command output to a file:

```sh
toralizer exec --command "curl http://example.com" --output output.txt
```

## Example

```sh
toralizer exec --command "curl http://example.com" --verbose --timeout 45s --proxy "socks5://127.0.0.1:9150" --output result.txt
```

This command will execute `curl http://example.com` through the Tor network with verbose output, a timeout of 45 seconds, using a custom proxy address, and redirect the output to `result.txt`.

## License

This project is licensed under the MIT License. See the [`LICENSE`](LICENSE) file for details.
