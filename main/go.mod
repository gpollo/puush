module puush/main

require (
	github.com/lib/pq v1.5.2 // indirect
	puush/database v0.0.0
    puush/server v0.0.0
)

replace puush/database => ../database

replace puush/server => ../server

go 1.13
