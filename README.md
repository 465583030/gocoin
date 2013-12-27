Gocoin is a full bitcoin client solution (node + wallet) written in Go language (golang).

The two basic components of the software are:

* **client** - a bitcoin node that must be connected to Internet
* **wallet** - a wallet app, that is designed to be used offline

# Webpage
The official webpage of the project:

* http://www.assets-otc.com/gocoin

On that webpage you can find all the information from this file, plus much much more (e.g. *User Manual*).



# Requirements

## Hardware
It is recommended to have at least 4GB of system memory on the PC where you run the online client node.
Because of the required memory space, the node will likely crash on a 32-bit system, so build it using 64-bit Go compiler.
The entire block chain is stored in one large file, so your file system must support files larger than 4GB.

The wallet app has very little requirements and should work on any platform with a working Go compiler.
For security reasons, use an encrypted swap file.
If you decide to store a password in the `.secret` file, do it on an encrypted disc.

## Software
Since no binaries are provided, in order to build Gocoin youself, you will need the following tools installed in your system:

* **Go** - http://golang.org/doc/install
* **Git** - http://git-scm.com/downloads
* **Mercurial** - http://mercurial.selenic.com/

If they are all properly installed you should be able to execute `go`, `git` and `hg` from your OS's command prompt without a need to specify their full path.

Note: Git and Mercurial are needed only for the automatic `go get` command to work. You can replace `go get` with some manual steps and then you do  not need these two tools. Read more at Gocoin's webpage.


# Building

## Download sources
Two extra  packages are needed, that are not included in the default set of Go libraries.
You need to download them, before building Gocoin.

	go get code.google.com/p/go.crypto/ripemd160
	go get code.google.com/p/snappy-go/snappy

You can also use `go get` to fetch the gocoin sources from GitHub for you:

	go get github.com/piotrnar/gocoin

Make sure that the all sources are placed in a proper location within your GOPATH folder, before compiling them (`go get` should take care of this).

## Compile client
Go to the `client/` folder and execute `go build` there.

## Compile wallet
Go to the `wallet/` folder and execute `go build` there.

If it fails on Windows, it is most likely because you do not have a compatible C compiler installed.
In such case either install MinGW for your host arch (32 or 64 bit), or just delete `hidepass_windows.go` and redo `go build`.


# Pull request
I am sorry to inform you that I will not merge in any pull requests.
The reason is that I want to stay the only author of this software and therefore the only holder of the copy rights.
I could have told you that I can pull your changes, if you waive the rights to your pieces of code, but that would be just rude.
So please fork and develop your own repo, if you want your code in.
Again, sorry about that.