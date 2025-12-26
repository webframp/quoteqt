import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const rateLimitedRate = new Rate('rate_limited');
const BASE_URL = 'http://localhost:8000';

export const options = {
  vus: 10,
  duration: '10s',
};

export default function () {
  const res = http.get(`${BASE_URL}/api/quote`);
  
  rateLimitedRate.add(res.status === 429);
  
  check(res, {
    'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  });
}
