import http from "k6/http";
import { check, fail } from "k6";
import exec from "k6/execution";
import { Counter } from "k6/metrics";

const baseUrl = __ENV.BASE_URL;
const email = __ENV.LOAD_TEST_EMAIL;
const password = __ENV.LOAD_TEST_PASSWORD;
const planId = __ENV.LOAD_TEST_PLAN_ID;

if (!baseUrl || !email || !password || !planId) {
  fail("BASE_URL and LOAD_TEST_* variables are required");
}

const allowedBaseUrls = new Set([
  "http://localhost:3000",
  "http://127.0.0.1:3000",
  "http://host.docker.internal:3000",
]);
if (!allowedBaseUrls.has(baseUrl)) {
  fail(`Phase 0 refuses non-local target: ${baseUrl}`);
}

const unexpected5xx = new Counter("unexpected_5xx");

export const options = {
  scenarios: {
    auth_probe: {
      executor: "per-vu-iterations",
      exec: "authProbe",
      vus: 2,
      iterations: 1,
      maxDuration: "30s",
    },
    concurrent_complete: {
      executor: "shared-iterations",
      exec: "concurrentComplete",
      vus: 1,
      iterations: 1,
      startTime: "3s",
      maxDuration: "30s",
    },
  },
  thresholds: {
    checks: ["rate==1"],
    unexpected_5xx: ["count==0"],
  },
};

function login() {
  const response = http.post(
    `${baseUrl}/api/auth/sign-in/email`,
    JSON.stringify({ email, password }),
    { headers: { "Content-Type": "application/json" }, redirects: 0 }
  );
  if (response.status >= 500) unexpected5xx.add(1);
  check(response, {
    "login succeeds": (result) => result.status === 200,
  });
  return response;
}

export function authProbe() {
  login();
  const response = http.get(`${baseUrl}/api/study-plans`);
  if (response.status >= 500) unexpected5xx.add(1);
  check(response, {
    "VU keeps its authenticated cookie": (result) => result.status === 200,
  });
}

export function concurrentComplete() {
  login();
  const request = [
    "POST",
    `${baseUrl}/api/study-plans/${planId}/complete`,
    JSON.stringify({ minutes: 10 }),
    { headers: { "Content-Type": "application/json" } },
  ];
  const responses = http.batch([request, request]);
  const statuses = responses.map((response) => response.status).sort();

  for (const response of responses) {
    if (response.status >= 500) unexpected5xx.add(1);
  }

  check(statuses, {
    "concurrent completion returns one 201 and one controlled 409": (values) =>
      values[0] === 201 && values[1] === 409,
  });

  console.log(
    `concurrent completion statuses=${statuses.join(",")} iteration=${exec.scenario.iterationInTest}`
  );
}
