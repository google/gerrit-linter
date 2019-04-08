FROM openjdk:8-jre

WORKDIR /app

# ADD /app/google-java-format.jar

# https://github.com/google/google-java-format/releases/download/google-java-format-1.7/google-java-format-1.7-all-deps.jar
COPY google-java-format-1.7-all-deps.jar /usr/bin/google-java-format-all-deps.jar

# go build github.com/google/fmtserver/cmd/fmtserver
COPY fmtserver /usr/bin/fmtserver

# go get github.com/bazelbuild/buildtools/buildifier
COPY buildifier /usr/bin/buildifier

# ??
COPY gofmt /usr/bin/gofmt

EXPOSE 80
CMD ["fmtserver",  "-port=80"]
