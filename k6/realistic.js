import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter, Trend } from 'k6/metrics';

// Custom metrics
const rateLimitedRate = new Rate('rate_limited');
const noResultsRate = new Rate('no_results');
const quotesServed = new Counter('quotes_served');

const BASE_URL = 'http://localhost:8000';

export const options = {
  scenarios: {
    // Realistic Nightbot usage: ~2 requests per minute per channel
    nightbot_realistic: {
      executor: 'constant-arrival-rate',
      rate: 5,  // 5 requests per second across all channels
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 5,
      exec: 'nightbotRequest',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<50'],  // 95% under 50ms
    rate_limited: ['rate<0.1'],        // Less than 10% rate limited
    http_req_failed: ['rate<0.01'],    // Less than 1% errors
  },
};

const channels = ['beastyqt', 'marinelord', 'viper', 'theviper', 'hera'];
const civs = ['hre', 'french', 'english', 'mongols', 'delhi', 'chinese', 'rus', 'abbasid'];

export function nightbotRequest() {
  const channel = channels[Math.floor(Math.random() * channels.length)];
  const civ = civs[Math.floor(Math.random() * civs.length)];
  const vsCiv = civs[Math.floor(Math.random() * civs.length)];
  
  const headers = {
    'Nightbot-Channel': `name=${channel}&displayName=${channel}&provider=twitch&providerId=${Math.floor(Math.random() * 100000)}`,
    'Nightbot-User': `name=viewer${Math.floor(Math.random() * 1000)}&displayName=Viewer&provider=twitch&userLevel=regular`,
  };

  let res;
  const requestType = Math.random();
  
  if (requestType < 0.5) {
    // 50% random quote
    res = http.get(`${BASE_URL}/api/quote`, { headers, tags: { endpoint: 'quote' } });
  } else if (requestType < 0.8) {
    // 30% civ-specific quote
    res = http.get(`${BASE_URL}/api/quote?civ=${civ}`, { headers, tags: { endpoint: 'quote_civ' } });
  } else {
    // 20% matchup tip
    res = http.get(`${BASE_URL}/api/matchup?civ=${civ}&vs=${vsCiv}`, { headers, tags: { endpoint: 'matchup' } });
  }

  // Track rate limiting
  const isRateLimited = res.status === 429;
  rateLimitedRate.add(isRateLimited);
  
  check(res, {
    'status is valid': (r) => [200, 400, 429].includes(r.status),
    'response has body': (r) => r.body && r.body.length > 0,
  });

  // Track results
  if (!isRateLimited) {
    if (res.body.includes('No quotes') || res.body.includes('No tips')) {
      noResultsRate.add(1);
    } else {
      noResultsRate.add(0);
      quotesServed.add(1);
    }
  }
  
  // Small random delay to simulate real usage
  sleep(0.1 + Math.random() * 0.2);
}
