#!/usr/bin/env bash

# This script patches the s2i Tekton task, so it recognizes registry.default.svc.cluster.local:5000 as insecure.

echo "Patching s2i Tekton task to use 'registry.default.svc.cluster.local:5000' as an insecure registry."

patch pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml <<EOF
diff --git a/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml b/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml
index a6973d70..f2bdb5d6 100644
--- a/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml
+++ b/pkg/pipelines/resources/tekton/task/func-s2i/0.1/func-s2i.yaml
@@ -102,6 +102,8 @@ spec:
       image: quay.io/buildah/stable:v1.27.0
       workingDir: /gen-source
       script: |
+        export BUILD_REGISTRY_SOURCES='{"insecureRegistries": ["registry.default.svc.cluster.local:5000"]}'
+
         TLS_VERIFY_FLAG=""
         if [ "\$(params.TLSVERIFY)" = "false" ] || [ "\$(params.TLSVERIFY)" = "0" ]; then
           TLS_VERIFY_FLAG="--tls-verify=false"
EOF
