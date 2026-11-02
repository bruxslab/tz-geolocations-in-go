// Run using the pinned reference API's tsx installation; see README.
// Executes original Hono handlers against their actual SQLite migration/seed.
import { DatabaseSync } from 'node:sqlite';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';
const root=process.env.REFERENCE_DIR;
if(!root) throw new Error('REFERENCE_DIR is required');
const source='d835ea318dbb17f789d4b7195b600e2e07e20f1e';
const seed=readFileSync(resolve(root,'api/src/db/seed.sql'),'utf8');
const db=new DatabaseSync(':memory:');
db.exec(readFileSync(resolve(root,'api/src/db/migrations/0000_init.sql'),'utf8'));
db.exec(seed);
const binding={prepare(sql:string){let args:any[]=[];return {bind(...values:any[]){args=values;return this},async first(){return db.prepare(sql).get(...args)??null},async all(){return {results:db.prepare(sql).all(...args)}}}}};
const {default:app}=await import(pathToFileURL(resolve(root,'api/src/index.ts')).href);
const paths=[
 '/', '/v1/regions','/v1/regions/arusha','/v1/regions/dar-es-salaam','/v1/regions/dar-es-salaam/districts',
 '/v1/regions/unknown','/v1/regions/unknown/districts','/v1/regions/INVALID',
 '/v1/districts/ilala','/v1/districts/ilala/wards','/v1/districts/arumeru/wards','/v1/districts/unknown','/v1/districts/unknown/wards','/v1/districts/INVALID',
 '/v1/wards/buguruni','/v1/wards/maweni','/v1/wards/buguruni/streets','/v1/wards/buguruni/streets?limit=1&offset=1',
 '/v1/wards/buguruni/streets?limit=0&offset=-1','/v1/wards/buguruni/streets?limit=9999','/v1/wards/buguruni/streets?limit=-9',
 '/v1/wards/buguruni/streets?limit=garbage&offset=garbage','/v1/wards/buguruni/streets?limit=0x2&offset=1e1',
 '/v1/wards/buguruni/streets?limit=2&offset=999999','/v1/wards/unknown','/v1/wards/unknown/streets','/v1/wards/INVALID',
 '/v1/streets/1','/v1/streets/75061','/v1/streets/0','/v1/streets/abc','/v1/streets/1.5','/v1/streets/999999','/v1/streets/1e0','/v1/streets/0x1',
 '/v1/postcodes/12102','/v1/postcodes/23306','/v1/postcodes/99999','/v1/postcodes/abc','/v1/postcodes/1234',
 '/v1/search','/v1/search?q=a&type=invalid','/v1/search?q=%27%3B--','/v1/search?q=arusha&type=region','/v1/search?q=buguruni&type=ward',
 '/v1/search?q=nonexistentzzzz','/v1/search?q=buguruni&type=ward&limit=0','/v1/search?q=buguruni&type=ward&limit=-1',
 '/v1/autocomplete','/v1/autocomplete?q=a','/v1/autocomplete?q=--','/v1/autocomplete?q=nonexistentzzzz',
 '/v1/stats','/v1/unknown','/v1/regions/',
];
// Broader search results may change because PostgreSQL ranking and street indexing
// are approved differences. Capture the source results as evidence, not new goldens.
const differing=['/v1/search?q=buguruni','/v1/search?q=malapa&type=street','/v1/search?q=dar%20es%20salaam','/v1/autocomplete?q=bug','/v1/autocomplete?q=arusha'];
const cases=[];
for(const path of [...paths,...differing]){
 const response=await app.request(path,{headers:{'CF-Connecting-IP':`fixture-${cases.length}`}}, {DB:binding});
 cases.push({path,status:response.status,comparison:differing.includes(path)?'search':'exact',body:await response.json(),cache_control:response.headers.get('cache-control')});
}
writeFileSync('tests/contract/testdata/reference.json',JSON.stringify({source_commit:source,seed_sha256:createHash('sha256').update(seed).digest('hex'),cases},null,2)+'\n');
const offline=await import(pathToFileURL(resolve(root,'packages/tz-locations/src/index.ts')).href);
const calls=[['getRegions',[]],['getRegion',['arusha']],['getRegion',['unknown']],['getDistricts',[]],['getDistricts',['dar-es-salaam']],['getDistrictsByRegion',['dar-es-salaam']],['getDistrict',['ilala']],['getDistrict',['unknown']],['getWards',['dar-es-salaam','ilala']],['getWardsByDistrict',['dar-es-salaam','ilala']],['getWard',['maweni']],['getWard',['unknown']],['getStreets',['dar-es-salaam','ilala','buguruni']],['getByPostcode',['12102']],['getByPostcode',['99999']],['searchLocations',['buguruni']],['searchLocations',['',{limit:3}]],['searchLocations',['a',{type:'street',limit:4}]],['searchLocations',['a',{type:'ward',limit:0}]],['getStats',[]]] as const;
writeFileSync('tests/contract/testdata/offline.json',JSON.stringify({source_commit:source,cases:calls.map(([fn,args])=>({fn,args,result:offline[fn](...args)??null}))},null,2)+'\n');
console.log(`Recorded ${cases.length} API and ${calls.length} offline cases from ${source}`);
db.close();
