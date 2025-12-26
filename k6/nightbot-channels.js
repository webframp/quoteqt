import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter } from 'k6/metrics';

const rateLimitedRate = new Rate('rate_limited');
const quotesServed = new Counter('quotes_served');

const BASE_URL = 'http://localhost:8000';

const channels = [
  { name: 'beastyqt', providerId: '12345' },
  { name: 'marinelord', providerId: '23456' },
  { name: 'viper', providerId: '34567' },
  { name: 'hera', providerId: '45678' },
  { name: 'drongo', providerId: '56789' },
];

const civs = ['hre', 'french', 'english', 'mongols', 'delhi', 'chinese', 'rus', 'abbasid'];

export const options = {
  scenarios: {
    // Realistic: each channel makes ~10 requests over 60 seconds
    // That's about 1 request every 6 seconds per channel = 10/min (well under 30/min limit)
    channel_traffic: {
      executor: 'per-vu-iterations',
      vus: 5,
      iterations: 10,  // 10 requests per channel
      maxDuration: '2m',
      exec: 'channelRequest',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<50'],
    rate_limited: ['rate<0.1'],  // Less than 10% rate limited
  },
};

export function channelRequest() {
  const channel = channels[__VU - 1] || channels[0];
  const civ = civs[Math.floor(Math.random() * civs.length)];
  const vsCiv = civs[Math.floor(Math.random() * civs.length)];
  
  const headers = {
    'Nightbot-Channel': `name=${channel.name}&displayName=${channel.name}&provider=twitch&providerId=${channel.providerId}`,
    'Nightbot-User': `name=viewer${Math.floor(Math.random() * 1000)}&displayName=Viewer&provider=twitch&userLevel=regular`,
  };

  let res;
  const requestType = Math.random();
  
  if (requestType < 0.6) {
    res = http.get(`${BASE_URL}/api/quote`, { headers, tags: { channel: channel.name } });
  } else if (requestType < 0.85) {
    res = http.get(`${BASE_URL}/api/quote?civ=${civ}`, { headers, tags: { channel: channel.name } });
  } else {
    res = http.get(`${BASE_URL}/api/matchup?civ=${civ}&vs=${vsCiv}`, { headers, tags: { channel: channel.name } });
  }

  const isRateLimited = res.status === 429;
  rateLimitedRate.add(isRateLimited);
  
  check(res, {
    'status is valid': (r) => [200, 400, 429].includes(r.status),
  });

  if (!isRateLimited && !res.body.includes('No quotes') && !res.body.includes('No tips')) {
    quotesServed.add(1);
  }
  
  // More realistic delay: 4-8 seconds between commands
  // Real streams have sporadic command usage, not constant
  sleep(4 + Math.random() * 4);
}
