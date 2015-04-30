#!/bin/bash

. ./config.sh

start_suite "Create a container then start it"

weave_on $HOST1 reset
weave_on $HOST1 launch
assert_raises "docker_on $HOST1 ps | grep weave" 0

docker_on $HOST1 rm -f c1 || true
docker_on $HOST1 create -e WEAVE_CIDR=10.2.6.5/24 --name=c1 -t ubuntu
weave_on $HOST1 start c1
ok=$(docker -H tcp://$HOST1:2375 exec -i c1 sh -c "ifconfig | grep ethwe")
assert "test -n \"$ok\" && echo pass" "pass"

docker_on $HOST1 rm -f c1 || true

end_suite
