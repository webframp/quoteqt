import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter, Trend } from 'k6/metrics';

// Custom metrics
const rateLimitedRate = new Rate('rate_limited');
const noResultsRate = new Rate('no_results');
const quotesServed = new Counter('quotes_served');
const responseTime = new Trend('response_time_ms');

const BASE_URL = 'http://localhost:8000';

// Scenarios configuration
export const options = {
  scenarios: {
    // Scenario 1: Normal API usage
    normal_usage: {
      executor: 'constant-vus',
      vus: 5,
      duration: '30s',
      exec: 'normalUsage',
      tags: { scenario: 'normal' },
    },
    // Scenario 2: Burst traffic (triggers rate limiting)
    burst_traffic: {
      executor: 'constant-vus',
      vus: 20,
      duration: '10s',
      startTime: '35s',
      exec: 'burstTraffic',
      tags: { scenario: 'burst' },
    },
    // Scenario 3: Nightbot simulation
    nightbot_sim: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: '20s',
      preAllocatedVUs: 10,
      startTime: '50s',
      exec: 'nightbotSimulation',
      tags: { scenario: 'nightbot' },
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<100'], // 95% of requests under 100ms
    rate_limited: ['rate<0.5'],        // Less than 50% rate limited
    http_req_failed: ['rate<0.01'],    // Less than 1% errors
  },
};

// Scenario 1: Normal usage pattern
export function normalUsage() {
  const responses = [
    http.get(`${BASE_URL}/api/quote`),
    http.get(`${BASE_URL}/api/quote?civ=hre`),
    http.get(`${BASE_URL}/api/quote?civ=french`),
  ];

  for (const res of responses) {
    checkResponse(res);
    responseTime.add(res.timings.duration);
  }

  sleep(1);
}

// Scenario 2: Burst traffic to trigger rate limiting
export function burstTraffic() {
  const res = http.get(`${BASE_URL}/api/quote`);
  
  const isRateLimited = res.status === 429;
  rateLimitedRate.add(isRateLimited);
  
  check(res, {
    'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  });
  
  responseTime.add(res.timings.duration);
}

// Scenario 3: Simulate Nightbot requests
export function nightbotSimulation() {
  const channels = ['streamer1', 'streamer2', 'streamer3'];
  const civs = ['hre', 'french', 'english', 'mongols', 'delhi'];
  
  const channel = channels[Math.floor(Math.random() * channels.length)];
  const civ = civs[Math.floor(Math.random() * civs.length)];
  const vsCiv = civs[Math.floor(Math.random() * civs.length)];
  
  const headers = {
    'Nightbot-Channel': `name=${channel}&displayName=${channel}&provider=twitch&providerId=12345`,
    'Nightbot-User': 'name=viewer1&displayName=Viewer1&provider=twitch&providerId=67890&userLevel=regular',
  };

  // Mix of quote and matchup requests
  let res;
  if (Math.random() > 0.5) {
    res = http.get(`${BASE_URL}/api/quote?civ=${civ}`, { headers });
  } else {
    res = http.get(`${BASE_URL}/api/matchup?civ=${civ}&vs=${vsCiv}`, { headers });
  }

  checkResponse(res);
  responseTime.add(res.timings.duration);
  
  // Track no results
  if (res.body.includes('No quotes') || res.body.includes('No tips')) {
    noResultsRate.add(1);
  } else {
    noResultsRate.add(0);
    quotesServed.add(1);
  }
}

function checkResponse(res) {
  const isRateLimited = res.status === 429;
  rateLimitedRate.add(isRateLimited);
  
  check(res, {
    'status is 200, 400, or 429': (r) => [200, 400, 429].includes(r.status),
    'response has body': (r) => r.body && r.body.length > 0,
  });
}
