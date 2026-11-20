const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true, args: ['--ignore-certificate-errors'] });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  page.on('console', msg => console.log('BROWSER CONSOLE:', msg.text()));
  page.on('request', request => {
    if (request.url().includes('12345')) {
      console.log('>> FARO REQUEST:', request.method(), request.url());
    }
  });
  page.on('response', response => {
    if (response.url().includes('12345')) {
      console.log('<< FARO RESPONSE:', response.status(), response.url());
    }
  });

  await page.goto('http://127.0.0.1:31111');
  
  // Wait a bit for Faro to initialize and send payloads
  await page.waitForTimeout(5000);
  
  await browser.close();
})();
