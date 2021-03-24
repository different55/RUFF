Ruff provides a pop-up web server to Retrieve/Upload Files Fast over LAN,
inspired by WOOF (Web Offer One File) by Simon Budig.

It's based on the idea that not every device has <insert neat file transfer
tool here>, but just about every device that can network has an HTTP client,
making a hyper-simple HTTP server a viable option for file transfer with
zero notice or setup as long as *somebody* has a copy of RUFF.

Why create RUFF when WOOF exists? WOOF is no longer in the debian repos and
it's easier to `go get` a tool than it is to hunt down Simon's website for
the latest copy.

## Installation

`go get git.tilde.town/diff/ruff`

## Usage

Assuming $GOPATH is in $PATH:

`ruff "cool thing.jpg" # to send a cool file`

and

`ruff -u # to receive a cool file`
