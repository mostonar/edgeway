const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // Login
  await page.goto('https://192.168.20.167');
  await page.click('button:has-text("Sign in")');
  await page.click('text="GitHub"');
  await page.fill('#username', 'umair.ims19@gmail.com');
  await page.fill('#password', 'xoGithub2Svzlds');
  await page.click('input[name="commit"]');

  // Wait for Help dialog to appear
  await page.waitForSelector('text="Getting started"', { timeout: 10000 });
  console.log('Help dialog appeared');

  // Click OK button
  await page.click('button:has-text("Ok")');
  console.log('Clicked OK button');

  // Check if it disappears
  try {
    await page.waitForSelector('text="Getting started"', { state: 'hidden', timeout: 3000 });
    console.log('SUCCESS: Dialog disappeared');
  } catch (e) {
    console.log('FAILURE: Dialog is still visible');
  }

  await browser.close();
})();
