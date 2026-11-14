// Explicitly opt-in bootstrap only: never modify the external DNS provider or TLS.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const Esa = require('@alicloud/esa20240910');
const { Config } = require('@alicloud/openapi-client');

async function main() {
  assert(process.env.ESA_ACCESS_KEY_ID && process.env.ESA_ACCESS_KEY_SECRET, 'Missing ESA credentials');
  const client = new Esa.default(new Config({
    accessKeyId: process.env.ESA_ACCESS_KEY_ID,
    accessKeySecret: process.env.ESA_ACCESS_KEY_SECRET,
    endpoint: 'esa.cn-hangzhou.aliyuncs.com',
    regionId: 'cn-hangzhou',
  }));
  const name = 'xiaoshae-blog';
  const recordName = 'blog.xiaoshae.cn';
  const sites = await client.listSites(new Esa.ListSitesRequest({
    siteName: 'xiaoshae.cn', siteSearchType: 'exact', pageNumber: 1, pageSize: 50,
  }));
  const site = sites.body.sites?.find(item => item.siteName === 'xiaoshae.cn');
  assert(site && site.status === 'active', 'xiaoshae.cn is not an active ESA site');
  const related = await client.listRoutineRelatedRecords(new Esa.ListRoutineRelatedRecordsRequest({ name }));
  if (!related.body.relatedRecords?.some(record => record.recordName === recordName)) {
    // Do not silently replace an existing record owned by another service.
    const existing = await client.listRecords(new Esa.ListRecordsRequest({ siteId: site.siteId, recordName, pageNumber: 1, pageSize: 100 }));
    assert(!existing.body.records?.some(record => record.recordName === recordName),
      'An ESA DNS record already exists for blog.xiaoshae.cn; inspect it before binding');
    await client.createRoutineRelatedRecord(new Esa.CreateRoutineRelatedRecordRequest({ name, siteId: site.siteId, recordName }));
  }
  const records = await client.listRecords(new Esa.ListRecordsRequest({ siteId: site.siteId, recordName, pageNumber: 1, pageSize: 100 }));
  let record = records.body.records?.find(item => item.recordName === recordName);
  const bindings = await client.listRoutineRelatedRecords(new Esa.ListRoutineRelatedRecordsRequest({ name }));
  const binding = bindings.body.relatedRecords?.find(item => item.recordName === recordName);
  assert(binding, 'Domain binding was not found after creation');
  const detail = await client.getRecord(new Esa.GetRecordRequest({ siteId: site.siteId, recordId: binding.recordId }));
  record = detail.body.recordModel || record;
  console.log(JSON.stringify({ siteId: site.siteId, binding, record: record && { recordName: record.recordName, recordCname: record.recordCname, recordId: record.recordId, recordType: record.recordType }, recordResponseFields: Object.keys(detail.body) }, null, 2));
  assert(record?.recordCname, 'Binding created, but no dedicated CNAME was returned; inspect the ESA console');
  const report = `## ESA Pages domain\n\n- Project: ${name}\n- Domain: ${recordName}\n- External DNS type: CNAME\n- External DNS host: blog\n- External DNS value: ${record.recordCname}\n\nExternal DNS and TLS were not modified.\n`;
  console.log(report);
  if (process.env.GITHUB_STEP_SUMMARY) fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY, report);
}

main().catch(error => {
  // Never dump SDK request objects: they can contain authorization headers.
  console.error(`ESA domain setup failed: ${error.code || 'Error'}: ${error.message}`);
  process.exitCode = 1;
});
