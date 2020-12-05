module puush/main

require (
	puush/database v0.0.0
	puush/server v0.0.0
)

replace puush/database => ../database

replace puush/server => ../server

go 1.13
