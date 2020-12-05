module puush/server

require (
	github.com/lib/pq v1.9.0 // indirect
	puush/database v0.0.0
)

replace puush/database => ../database

go 1.13
