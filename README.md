# BlackChain Labs

BlackChain Labs builds decentralized infrastructure for edge computing, mesh networking, and AI-driven economic systems.

### 🌐 Links

Website  
https://BlackChainLabsLLC.com

Node Network  
https://NodeTheGlobe.com

GitHub  
https://github.com/BlackChainLabsLLC

# BlackChain Node

BlackChain Node is the core infrastructure daemon for the BlackChain protocol — a decentralized mesh-network blockchain designed for resilient peer-to-peer infrastructure and edge devices.

The project is maintained by BlackChain Labs LLC and forms the foundation for a broader ecosystem of decentralized applications and services.

---

## Overview

BlackChain is designed to operate across distributed networks of edge devices, enabling communities and organizations to operate their own infrastructure without dependence on centralized providers.

The node software provides the runtime responsible for:

• Mesh peer discovery  
• Transaction validation and signing  
• Validator identity and voting  
• Deterministic state hashing  
• Snapshot synchronization  
• Secure TLS transport between nodes

Together these systems form a lightweight but powerful blockchain network capable of operating on hardware ranging from cloud servers to edge devices.

---

## Repository Structure

cmd/ – Executable node binaries and CLI tools  
internal/ – Core protocol, consensus, networking, and state logic  
config/ – Node and mesh configuration templates  
scripts/ – Deployment and verification scripts  
docs/ – Protocol documentation and architecture notes  

---

## Key Capabilities

### Mesh Networking
Nodes automatically discover peers and propagate blocks across a distributed mesh topology.

### Deterministic State
Balances and network state are maintained using deterministic hashing and snapshot verification.

### Validator Consensus
Validators produce blocks, sign state transitions, and vote to finalize network history.

### Secure Communication
All node communications use authenticated TLS transport to ensure secure network connectivity.

---

## Running a Node

Clone the repository:

git clone https://github.com/BlackChainLabsLLC/blackchain-node.git

Build the daemon:

go build ./cmd/blacknetd

Run the node:

./blacknetd

More configuration details can be found in the config directory.

---

## Ecosystem

BlackChain Node is the foundational infrastructure layer of the BlackChain Labs ecosystem.

Projects built on or alongside the protocol include:

• Node™ — decentralized mesh internet infrastructure  
• Nexerra — autonomous AI economic simulation network  
• RollPay — decentralized settlement and payment layer  
• CivIntel — civilian intelligence and coordination network  
• EduChain — blockchain education funding infrastructure  
• MLP — Meta Liquidity Protocol for decentralized markets  

---

## Vision

BlackChain Labs aims to build decentralized infrastructure owned and operated by the communities that rely on it.

By combining mesh networking, blockchain verification, and edge computing, the BlackChain protocol enables a new model for community-owned digital infrastructure.

---

## License

MIT License
Copyright (c) 2026 BlackChain Labs LLC
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:
The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
