import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
  vus: 200,
  duration: '30s',
  thresholds: {
    http_req_duration: ['p(95)<10'],
    http_reqs: ['count>=150000'],
  },
};

const baseURL = __ENV.METADATA_BASE_URL || 'http://localhost:8082';

export default function () {
  const videoID = `bench-${__VU}-${__ITER}`;
  const create = http.post(`${baseURL}/videos`, JSON.stringify({ id: videoID }), {
    headers: { 'Content-Type': 'application/json' },
  });
  check(create, { 'create ok': (r) => r.status === 200 });

  const put = http.post(`${baseURL}/videos/${videoID}/segments?rend=720p&idx=0`, '{}', {
    headers: { 'Content-Type': 'application/json' },
  });
  check(put, { 'put ok': (r) => r.status === 200 });

  const get = http.get(`${baseURL}/videos/${videoID}/segments/loc?rend=720p&idx=0`);
  check(get, { 'get ok': (r) => r.status === 200 });

  sleep(0.01);
}
