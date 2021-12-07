# Streams to Websockets

Proof of concept code where a web socket can receive all updates on a [Redis Stream](https://redis.io/topics/streams-intro)



### Testing

Startup docker-compose

```
docker-compose up
```

Open a [web browser](http://localhost:8080/test)

Using Redis CLI add an entry to the test stream and watch the websocket test page update

```
$ for j in {1..10}; do
redis-cli xadd test_stream "*" key value${j} count $j time `date +%s.%N` user `whoami`
sleep 1
done
```
