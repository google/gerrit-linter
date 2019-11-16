#!/bin/sh
set -eux

if [[ -d .git ]]; then
  VERSION=-$(git show --pretty=format:%h -q)
fi
VERSION=$(date --iso-8601=minutes | tr -d ':' | sed 's|\+.*$||')${VERSION}

dest=gerrit-linter-${VERSION}
mkdir -p ${dest}

trap "rm -rf ${dest}" 'EXIT'

go build -o ${dest}/gerrit-linter  ./cmd/checker

if [[ ! -f  google-java-format.jar ]] ; then
  curl -Lo google-java-format.jar https://github.com/google/google-java-format/releases/download/google-java-format-1.7/google-java-format-1.7-all-deps.jar
fi

cp google-java-format.jar ${dest}/
chmod +x ${dest}/*.jar

go build -o ${dest}/buildifier github.com/bazelbuild/buildtools/buildifier

cp $(which gofmt) ${dest}/

chmod 755 ${dest}/*
tar cfz ${dest}.tar.gz ${dest}/
