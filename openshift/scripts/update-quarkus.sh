#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

update_pom() {

  declare -r pom="$1"
  declare -r version="$(curl -s "https://code.quarkus.redhat.com/api/platforms" | jq -r '.platforms[0].streams[0].releases[0].version')"

  sed -i 's#<maven.compiler.release>11</maven.compiler.release>#<maven.compiler.release>17</maven.compiler.release>#g' "${pom}"
  sed -i 's#<quarkus.platform.group-id>io.quarkus.platform</quarkus.platform.group-id>#<quarkus.platform.group-id>com.redhat.quarkus.platform</quarkus.platform.group-id>#g' "${pom}"
  sed -i "s#<quarkus.platform.version>.*</quarkus.platform.version>#<quarkus.platform.version>${version}</quarkus.platform.version>#g" "${pom}"
  sed -i '54i\ \ <repositories>\
    <repository>\
      <releases>\
        <enabled>true</enabled>\
      </releases>\
      <snapshots>\
        <enabled>false</enabled>\
      </snapshots>\
      <id>redhat</id>\
      <url>https://maven.repository.redhat.com/ga</url>\
    </repository>\
  </repositories>\
  <pluginRepositories>\
    <pluginRepository>\
      <releases>\
        <enabled>true</enabled>\
      </releases>\
      <snapshots>\
        <enabled>false</enabled>\
      </snapshots>\
      <id>redhat</id>\
      <url>https://maven.repository.redhat.com/ga</url>\
    </pluginRepository>\
  </pluginRepositories>' "${pom}"
}

main() {
  declare -r poms=("templates/quarkus/cloudevents/pom.xml" "templates/quarkus/http/pom.xml")
  local p
  for p in "${poms[@]}"; do
    update_pom "${p}"
  done
}

main "$@"
