import { describe, it, expect } from "bun:test";
import { readFileSync, readdirSync } from "fs";
import { join } from "path";
import { load } from "js-yaml";

const K8S_DIR = join(import.meta.dir, "..", "k8s");

function readYaml(filename: string): any {
  const content = readFileSync(join(K8S_DIR, filename), "utf-8");
  return load(content);
}

describe("Kubernetes Manifests", () => {
  describe("Namespace", () => {
    it("should define compute-commander namespace", () => {
      const ns = readYaml("namespace.yaml");
      expect(ns.apiVersion).toBe("v1");
      expect(ns.kind).toBe("Namespace");
      expect(ns.metadata.name).toBe("compute-commander");
    });
  });

  describe("WebSocket Deployment", () => {
    const deploy = readYaml("websocket-deployment.yaml");

    it("should be a Deployment", () => {
      expect(deploy.kind).toBe("Deployment");
      expect(deploy.apiVersion).toBe("apps/v1");
    });

    it("should have 1 replica", () => {
      expect(deploy.spec.replicas).toBe(1);
    });

    it("should be in compute-commander namespace", () => {
      expect(deploy.metadata.namespace).toBe("compute-commander");
    });

    it("should have resource limits", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.resources.limits).toBeDefined();
      expect(container.resources.requests).toBeDefined();
    });

    it("should have readiness probe", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.readinessProbe).toBeDefined();
      expect(container.readinessProbe.httpGet.path).toBe("/health");
      expect(container.readinessProbe.httpGet.port).toBe(8080);
    });

    it("should have liveness probe", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.livenessProbe).toBeDefined();
      expect(container.livenessProbe.httpGet.path).toBe("/health");
    });

    it("should expose port 8080", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.ports[0].containerPort).toBe(8080);
    });
  });

  describe("WebSocket Service", () => {
    const svc = readYaml("websocket-service.yaml");

    it("should be a ClusterIP service", () => {
      expect(svc.kind).toBe("Service");
      expect(svc.spec.type).toBe("ClusterIP");
    });

    it("should target websocket-server pods", () => {
      expect(svc.spec.selector.app).toBe("websocket-server");
    });

    it("should expose port 8080", () => {
      expect(svc.spec.ports[0].port).toBe(8080);
    });
  });

  describe("API Deployment", () => {
    const deploy = readYaml("api-deployment.yaml");

    it("should be a Deployment", () => {
      expect(deploy.kind).toBe("Deployment");
    });

    it("should have 2 replicas", () => {
      expect(deploy.spec.replicas).toBe(2);
    });

    it("should be in compute-commander namespace", () => {
      expect(deploy.metadata.namespace).toBe("compute-commander");
    });

    it("should have resource limits", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.resources.limits).toBeDefined();
      expect(container.resources.requests).toBeDefined();
    });

    it("should have readiness and liveness probes on /health", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.readinessProbe.httpGet.path).toBe("/health");
      expect(container.readinessProbe.httpGet.port).toBe(3000);
      expect(container.livenessProbe.httpGet.path).toBe("/health");
    });

    it("should reference WebSocket server URL in env", () => {
      const container = deploy.spec.template.spec.containers[0];
      const wsEnv = container.env.find(
        (e: any) => e.name === "WS_SERVER_URL"
      );
      expect(wsEnv).toBeDefined();
      expect(wsEnv.value).toContain("websocket-server");
    });
  });

  describe("API Service", () => {
    const svc = readYaml("api-service.yaml");

    it("should be a ClusterIP service", () => {
      expect(svc.kind).toBe("Service");
      expect(svc.spec.type).toBe("ClusterIP");
    });

    it("should target api-server pods", () => {
      expect(svc.spec.selector.app).toBe("api-server");
    });

    it("should expose port 3000", () => {
      expect(svc.spec.ports[0].port).toBe(3000);
    });
  });

  describe("HAProxy ConfigMap", () => {
    const cm = readYaml("haproxy-configmap.yaml");

    it("should be a ConfigMap", () => {
      expect(cm.kind).toBe("ConfigMap");
    });

    it("should contain haproxy.cfg", () => {
      expect(cm.data["haproxy.cfg"]).toBeDefined();
    });

    it("should use leastconn balancing", () => {
      expect(cm.data["haproxy.cfg"]).toContain("balance leastconn");
    });
  });

  describe("HAProxy Deployment", () => {
    const deploy = readYaml("haproxy-deployment.yaml");

    it("should mount config from ConfigMap", () => {
      const volume = deploy.spec.template.spec.volumes[0];
      expect(volume.configMap.name).toBe("haproxy-config");

      const mount =
        deploy.spec.template.spec.containers[0].volumeMounts[0];
      expect(mount.mountPath).toContain("haproxy.cfg");
    });

    it("should have resource limits", () => {
      const container = deploy.spec.template.spec.containers[0];
      expect(container.resources.limits).toBeDefined();
    });

    it("should expose ports 80, 8080, 9090", () => {
      const ports = deploy.spec.template.spec.containers[0].ports;
      const portNums = ports.map((p: any) => p.containerPort);
      expect(portNums).toContain(80);
      expect(portNums).toContain(8080);
      expect(portNums).toContain(9090);
    });
  });

  describe("HAProxy Service", () => {
    const svc = readYaml("haproxy-service.yaml");

    it("should be a LoadBalancer type", () => {
      expect(svc.spec.type).toBe("LoadBalancer");
    });

    it("should expose HTTP and WebSocket ports", () => {
      const ports = svc.spec.ports.map((p: any) => p.port);
      expect(ports).toContain(80);
      expect(ports).toContain(8080);
    });
  });

  describe("Kustomization", () => {
    const kust = readYaml("kustomization.yaml");

    it("should list all resources", () => {
      const resources = kust.resources;
      expect(resources).toContain("namespace.yaml");
      expect(resources).toContain("websocket-deployment.yaml");
      expect(resources).toContain("websocket-service.yaml");
      expect(resources).toContain("api-deployment.yaml");
      expect(resources).toContain("api-service.yaml");
      expect(resources).toContain("haproxy-configmap.yaml");
      expect(resources).toContain("haproxy-deployment.yaml");
      expect(resources).toContain("haproxy-service.yaml");
    });

    it("should target compute-commander namespace", () => {
      expect(kust.namespace).toBe("compute-commander");
    });
  });

  describe("All manifests", () => {
    it("should be valid YAML files", () => {
      const files = readdirSync(K8S_DIR).filter((f) => f.endsWith(".yaml"));
      for (const file of files) {
        const content = readFileSync(join(K8S_DIR, file), "utf-8");
        expect(() => load(content)).not.toThrow();
      }
    });
  });
});
