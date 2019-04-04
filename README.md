

FMTSERVER
=========

This is a style verifier intended to be used with the Gerrit checks
plugin.

It consists of the following components

   * fmtserver: an RPC server that reformats source code. It is
     intended to be run in a Docker container that is secured with
     gvisor.

   * checker: a daemon that contacts gerrit, and sends pending changes
     to fmtserver to validate correct formatting


TODO
====

   * handle file types (symlink) and deletions

   * use the full checker API

   * more formatters: clang-format, typescript, jsformat, ... ?

   * isolate each formatter run with a separate gvisor/docker
     container?


DISCLAIMER
==========
This is not an official Google product
