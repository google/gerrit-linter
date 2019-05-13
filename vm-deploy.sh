#!/bin/sh

set -eux

t=$(basename $1 .tar.gz)

tar fzx $t.tar.gz
rm -f gerrit-linter
ln -s $t gerrit-linter
