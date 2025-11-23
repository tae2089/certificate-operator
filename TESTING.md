# OrbStack 테스트 가이드

OrbStack에서 Certificate Operator를 테스트하기 위한 단계별 가이드입니다.

## 사전 요구사항

1. **OrbStack 설치 및 Kubernetes 활성화**
2. **cert-manager 설치**

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
```

cert-manager가 준비될 때까지 대기:
```bash
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager -n cert-manager
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager-webhook -n cert-manager
kubectl wait --for=condition=available --timeout=300s deployment/cert-manager-cainjector -n cert-manager
```

## 1. ClusterIssuer 생성

Self-signed ClusterIssuer 생성:

```bash
kubectl apply -f config/samples/clusterissuer_selfsigned.yaml
```

확인:
```bash
kubectl get clusterissuer
```

## 2. Operator 배포

### CRD 설치
```bash
make install
```

### Operator 실행 (로컬 모드)
```bash
make run
```

또는 Docker 이미지로 배포:
```bash
# 이미지 빌드
make docker-build IMG=certificate-operator:latest

# 클러스터에 배포
make deploy IMG=certificate-operator:latest
```

## 3. Certificate 생성 테스트

### 샘플 Certificate 생성

```bash
kubectl apply -f config/samples/certificate_test.yaml
```

### 상태 확인

```bash
# Certificate CR 확인
kubectl get certificates -n default

# 상세 정보 확인
kubectl describe certificate example-certificate -n default

# cert-manager Certificate 확인
kubectl get certificate -n default

# Secret 확인 (TLS 인증서가 저장됨)
kubectl get secret -n default
```

## 4. API 서버 테스트

Operator가 실행 중이면 API 서버가 포트 8080에서 실행됩니다.

### Port Forward 설정

```bash
# Operator가 Pod로 실행 중인 경우
kubectl port-forward -n certificate-operator-system deployment/certificate-operator-controller-manager 8080:8080

# 또는 로컬 실행 중이면 바로 접근 가능
```

### Health Check

```bash
curl http://localhost:8080/healthz
```

### Swagger UI 접근

브라우저에서 다음 URL 접속:
```
http://localhost:8080/swagger/index.html
```

### API 테스트

#### Certificate 생성
```bash
curl -X POST http://localhost:8080/api/v1/certificates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "api-test-cert",
    "namespace": "default",
    "domain": "api-test.local",
    "clusterIssuerName": "selfsigned-issuer"
  }'
```

#### Certificate 목록 조회
```bash
# 모든 네임스페이스
curl http://localhost:8080/api/v1/certificates

# default 네임스페이스만
curl http://localhost:8080/api/v1/namespaces/default/certificates
```

#### 특정 Certificate 조회
```bash
curl http://localhost:8080/api/v1/namespaces/default/certificates/api-test-cert
```

#### Certificate 업데이트
```bash
curl -X PUT http://localhost:8080/api/v1/namespaces/default/certificates/api-test-cert \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "updated-api-test.local"
  }'
```

#### Certificate 삭제
```bash
curl -X DELETE http://localhost:8080/api/v1/namespaces/default/certificates/api-test-cert
```

## 5. 로그 확인

### Operator 로그
```bash
# Pod로 실행 중인 경우
kubectl logs -n certificate-operator-system deployment/certificate-operator-controller-manager -f

# 로컬 실행 중이면 터미널에서 직접 확인
```

### cert-manager 로그
```bash
kubectl logs -n cert-manager deployment/cert-manager -f
```

## 6. 정리

### Certificate 삭제
```bash
kubectl delete -f config/samples/certificate_test.yaml
```

### Operator 정리
```bash
# CRD 및 리소스 삭제
make uninstall

# 배포된 경우
make undeploy
```

## 트러블슈팅

### Certificate가 Ready 상태가 되지 않는 경우

1. cert-manager Certificate 확인:
```bash
kubectl describe certificate -n default
```

2. cert-manager 로그 확인:
```bash
kubectl logs -n cert-manager deployment/cert-manager
```

3. Operator 로그 확인:
```bash
kubectl logs -n certificate-operator-system deployment/certificate-operator-controller-manager
```

### API 서버에 접근할 수 없는 경우

1. Operator가 실행 중인지 확인
2. Port forward가 설정되어 있는지 확인
3. `--enable-api-server=true` 플래그가 설정되어 있는지 확인
