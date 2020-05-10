FROM golang:latest AS builder
WORKDIR /puush
COPY . .
RUN cd main && go build -o puush-server

FROM golang:latest AS deploy
COPY --from=builder /puush/main/puush-server /usr/bin/
ENTRYPOINT /usr/bin/puush-server
