#!/bin/sh

latest=$(ls -1tr | grep tar.gz | tail -1)

scp vm-deploy.sh ${latest} ${IP}:
ssh ${IP} "sh vm-deploy.sh ${latest}"
