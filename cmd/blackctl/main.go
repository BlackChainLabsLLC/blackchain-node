package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "net"
    "os"
    "time"
)

func main() {
    addr := flag.String("addr", "10.0.0.100:55211", "RPC address (host:port)")
    raw := flag.Bool("raw", false, "print raw JSON instead of pretty output")
    flag.Parse()

    conn, err := net.Dial("tcp", *addr)
    if err != nil {
        fmt.Fprintf(os.Stderr, "dial error: %v\n", err)
        os.Exit(1)
    }
    defer conn.Close()

    // prevent infinite hangs
    conn.SetReadDeadline(time.Now().Add(2 * time.Second))

    data, err := io.ReadAll(conn)
    if err != nil {
        fmt.Fprintf(os.Stderr, "read error: %v\n", err)
    }

    if *raw {
        fmt.Printf("%s\n", data)
        return
    }

    var v any
    if err := json.Unmarshal(data, &v); err != nil {
        fmt.Printf("%s\n", data)
        return
    }

    pretty, _ := json.MarshalIndent(v, "", "  ")
    fmt.Println(string(pretty))
}

