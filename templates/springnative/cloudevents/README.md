# Java Native Image Sample Application

## Building

### With `pack`
```bash
pack build applications/native-image \
  --builder paketobuildpacks/builder:tiny \
  --env BP_NATIVE_IMAGE=true
```

### With the Spring Boot Maven Plugin
```
./mvnw -Dmaven.test.skip=true spring-boot:build-image
```

## Running
```bash
docker run --rm --tty --publish 8080:8080 applications/native-image
```

## Viewing

```bash
curl -s http://localhost:8080/actuator/health | jq .
```
