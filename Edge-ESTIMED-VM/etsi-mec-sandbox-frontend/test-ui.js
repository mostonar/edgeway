const { chromium } = require('playwright');
const fs = require('fs');

const SCRATCH = '/home/xflow/.gemini/antigravity-ide/brain/c305e248-1c6b-4c5a-8e06-2d4be782847e/scratch';

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  const consoleLogs = [];
  page.on('console', msg => {
    consoleLogs.push('[' + msg.type() + '] ' + msg.text());
  });

  try {
    console.log('1. Navigating to app...');
    await page.goto('https://192.168.20.167/', { waitUntil: 'networkidle' });

    console.log('2. Clicking SIGN IN...');
    await page.evaluate(() => {
      const btns = Array.from(document.querySelectorAll('button'));
      const btn = btns.find(b => b.textContent.trim() === 'SIGN IN');
      if (btn) btn.click();
      else throw new Error('SIGN IN button not found');
    });
    await page.waitForTimeout(1000);

    console.log('3. Clicking GitHub...');
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle', timeout: 15000 }),
      page.evaluate(() => {
        const btns = Array.from(document.querySelectorAll('button'));
        const btn = btns.find(b => b.textContent && b.textContent.includes('GitHub'));
        if (btn) btn.click();
        else throw new Error('GitHub button not found');
      })
    ]);
    console.log('   At GitHub: ' + page.url());

    console.log('4. Filling GitHub credentials...');
    await page.fill('#login_field', 'umair.ims19@gmail.com');
    await page.fill('#password', 'xoGithub2Svzlds');
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle', timeout: 15000 }),
      page.click('input[type="submit"]')
    ]);
    console.log('   After submit: ' + page.url());

    // Check for 2FA page
    if (page.url().includes('two-factor') || page.url().includes('sessions')) {
      console.log('   *** 2FA PAGE - Please approve on your phone! ***');
      await page.screenshot({ path: SCRATCH + '/pw_2fa.png' });
      // Wait up to 60s for 2FA approval and redirect
      try {
        await page.waitForURL('https://192.168.20.167/**', { timeout: 60000 });
        console.log('   2FA approved! Redirected to: ' + page.url());
      } catch(e) {
        // Maybe went to authorize page first
        console.log('   Current URL after wait: ' + page.url());
      }
    }

    // Handle GitHub authorize page
    if (page.url().includes('authorize') || page.url().includes('github.com')) {
      console.log('   At authorize page: ' + page.url());
      await page.screenshot({ path: SCRATCH + '/pw_authorize.png' });
      try {
        await page.click('button[name="authorize"]', { timeout: 5000 });
        await page.waitForURL('https://192.168.20.167/**', { timeout: 15000 });
      } catch(e) {
        console.log('   Waiting for redirect...');
        await page.waitForURL('https://192.168.20.167/**', { timeout: 30000 });
      }
    }

    console.log('   App URL: ' + page.url());
    await page.waitForTimeout(2000);
    await page.screenshot({ path: SCRATCH + '/pw_app_loaded.png' });

    console.log('5. Checking dialog state...');
    const dialogInfo = await page.evaluate(() => {
      const dialogs = document.querySelectorAll('.mdc-dialog');
      return Array.from(dialogs).map((d, i) => {
        const title = d.querySelector('.mdc-dialog__title');
        return {
          index: i,
          title: title ? title.textContent.trim() : '(none)',
          open: d.classList.contains('mdc-dialog--open'),
        };
      });
    });
    console.log('   All dialogs: ' + JSON.stringify(dialogInfo));

    console.log('6. Monitoring for 90s...');
    for (let i = 0; i < 30; i++) {
      await page.waitForTimeout(3000);
      const state = await page.evaluate(() => {
        const openDialogs = document.querySelectorAll('.mdc-dialog--open');
        return {
          openCount: openDialogs.length,
          titles: Array.from(openDialogs).map(d => {
            const t = d.querySelector('.mdc-dialog__title');
            return t ? t.textContent.trim() : '?';
          }),
          hasInit: document.body.textContent.includes('Initializing sandbox'),
          hasScrollLock: document.body.classList.contains('mdc-dialog-scroll-lock'),
        };
      });
      console.log('   t+' + ((i+1)*3) + 's: ' + JSON.stringify(state));
      
      if (!state.hasInit && state.openCount === 0 && !state.hasScrollLock) {
        console.log('   *** SUCCESS: No stuck dialogs! Sandbox loaded cleanly. ***');
        await page.screenshot({ path: SCRATCH + '/pw_success.png' });
        break;
      }
    }

    await page.screenshot({ path: SCRATCH + '/pw_final.png' });
    fs.writeFileSync(SCRATCH + '/pw_console.log', consoleLogs.join('\n'));
    console.log('Done.');

  } catch (e) {
    console.error('ERROR: ' + e.message);
    await page.screenshot({ path: SCRATCH + '/pw_error.png' }).catch(() => {});
    fs.writeFileSync(SCRATCH + '/pw_console.log', consoleLogs.join('\n'));
  }

  await browser.close();
})();
