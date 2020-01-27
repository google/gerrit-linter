# GERRITFMT

This is a style verifier intended to be used with the Gerrit checks
plugin.

## HOW TO USE

1. Install formatters:

```sh
go install github.com/bazelbuild/buildtools/buildifier
curl -o google-java-format.jar https://github.com/google/google-java-format/releases/download/google-java-format-1.7/google-java-format-1.7-all-deps.jar
```

1. Obtain an HTTP password, and put it in `testsite-auth`. The format is
   `username:secret`.


2. Register a checker

```sh
go run ./cmd/checker -auth_file=testsite-auth  --gerrit http://localhost:8080 \
  --language go --repo gerrit --register
```

3. Make sure the checker is there

```sh
go run ./cmd/checker -auth_file=testsite-auth  --gerrit http://localhost:8080 \
  --list
```

4. Start the server

```sh
go run ./cmd/checker -auth_file=testsite-auth  --gerrit http://localhost:8080
```



## DESIGN

For simplicity of deployment, the gerrit-linter checker is stateless. All the
necessary data is encoded in the checker UUID.


## TODO

   * handle file types (symlink) and deletions

   * more formatters: clang-format, typescript, jsformat, ... ?

   * isolate each formatter to run with a separate gvisor/docker
     container.

   * tests: the only way to test this reliably is to spin up a gerrit server,
     and create changes against the server.

   * Update the list of checkers periodically.

## SECURITY

This currently runs the formatters without sandboxing. Critical bugs (heap
overflow, buffer overflow) in formatters can be escalated to obtain the OAuth2
token used for authentication.

The currently supported formatters are written in Java and Go, so this should
not be an issue.


## DOCKER ON GCP

The following example shows how to build a Docker image hosted on GCP, in the
project `api-project-164060093628`.

```
VERSION=$(date --iso-8601=minutes | tr -d ':' | tr '[A-Z]' '[a-z]'| sed \
    's|\+.*$||')-$(git rev-parse --short HEAD)
NAME=gcr.io/api-project-164060093628/gerrit-linter:${VERSION}
docker build -t ${NAME} -f Dockerfile .
docker push ${NAME}
```

To deploy onto a GCP VM, configure the VM to have scope
`https://www.googleapis.com/auth/gerritcodereview`:

```sh
cloud beta compute instances set-scopes VM-NAME --scopes=https://www.googleapis.com/auth/gerritcodereview
```


## DISCLAIMER

This is not an official Google product
