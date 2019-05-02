

FMTSERVER
=========

This is a style verifier intended to be used with the Gerrit checks
plugin.

TODO
====

   * handle file types (symlink) and deletions

   * more formatters: clang-format, typescript, jsformat, ... ?

   * isolate each formatter to run with a separate gvisor/docker
     container.

   * tests: the only way to test this reliably is to spin up a gerrit server,
     and create changes against the server.


SECURITY
========

This currently runs the formatters without sandboxing. Critical bugs in
formatters can be escalated to obtain the OAuth2 token used for authentication.


DISCLAIMER
==========
This is not an official Google product
