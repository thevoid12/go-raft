## this is the readme of my raft algorithm implementation

<h3>example of curling to set the value</h3>
curl -X POST -H "Content-Type: application/json" -d '{"command":"set x=1"}' http://localhost:8001/client

<h3>command to run </h3>
# Terminal 1
./go-raft -id=0 -addr=localhost:8001

# Terminal 2
./go-raft -id=1 -addr=localhost:8002

# Terminal 3
./go-raft -id=2 -addr=localhost:8003

# Terminal 4
./go-raft -id=3 -addr=localhost:8004

# Terminal 5
./go-raft -id=4 -addr=localhost:8005
