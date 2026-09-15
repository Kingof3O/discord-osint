#!/usr/bin/env node

/**
 * Disboard -> discord.gg Automated Resolver
 *
 * Uses genuine Chrome via Playwright with persistent storage to bypass Cloudflare
 * TLS fingerprinting, handle NSFW/age gates, and capture clean discord.gg/xxxx invites.
 *
 * Usage:
 *   # 1. Resolve specific Disboard server ID(s) or URLs:
 *   node scripts/resolve-disboard.js 1164734637927571477
 *   node scripts/resolve-disboard.js https://disboard.org/server/1164734637927571477
 *
 *   # 2. Search Disboard by keyword/tag and auto-resolve top N servers:
 *   node scripts/resolve-disboard.js --search "crypto" --limit 10
 *
 *   # 3. Read a list of Disboard server URLs/IDs from file:
 *   node scripts/resolve-disboard.js -f disboard_servers.txt -o invites.txt
 */

const fs = require('fs');
const path = require('path');

async function main() {
  const args = process.argv.slice(2);
  let inputFile = null;
  let outputFile = 'invites.txt';
  let profileDir = path.join(__dirname, '..', '.chrome-disboard-profile');
  let headless = false;
  let searchQuery = null;
  let searchLimit = 15;
  let targets = [];

  // Parse CLI args
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '-f' || args[i] === '--file' || args[i] === '--input') {
      inputFile = args[++i];
    } else if (args[i] === '-o' || args[i] === '--output') {
      outputFile = args[++i];
    } else if (args[i] === '--headless') {
      headless = true;
    } else if (args[i] === '--profile') {
      profileDir = args[++i];
    } else if (args[i] === '-s' || args[i] === '--search' || args[i] === '--tag') {
      searchQuery = args[++i];
    } else if (args[i] === '-l' || args[i] === '--limit') {
      searchLimit = parseInt(args[++i], 10) || 15;
    } else if (args[i] === '-h' || args[i] === '--help') {
      printHelp();
      process.exit(0);
    } else if (!args[i].startsWith('-')) {
      targets.push(args[i]);
    }
  }

  // Load from input file if specified
  if (inputFile) {
    if (!fs.existsSync(inputFile)) {
      console.error(`[-] Input file not found: ${inputFile}`);
      process.exit(1);
    }
    const lines = fs.readFileSync(inputFile, 'utf-8').split('\n');
    for (let line of lines) {
      line = line.trim();
      if (line && !line.startsWith('#')) {
        targets.push(line);
      }
    }
  }

  if (targets.length === 0 && !searchQuery) {
    console.error('[-] No Disboard targets or search query provided.');
    printHelp();
    process.exit(1);
  }

  let playwright;
  try {
    playwright = require('playwright');
  } catch {
    try {
      playwright = await import('playwright');
    } catch {
      console.error('[-] Playwright is not installed. Please run: npm install -D playwright');
      process.exit(1);
    }
  }

  console.log(`======================================================================`);
  console.log(`[*] Disboard -> discord.gg Browser Resolver`);
  console.log(`[*] Profile: ${profileDir}`);
  console.log(`[*] Output : ${outputFile}`);
  console.log(`======================================================================\n`);

  // Launch persistent context with installed Google Chrome
  const browserContext = await playwright.chromium.launchPersistentContext(profileDir, {
    channel: 'chrome',
    headless: headless,
    viewport: { width: 1280, height: 850 },
    args: [
      '--disable-blink-features=AutomationControlled',
      '--no-default-browser-check'
    ]
  });

  const page = browserContext.pages()[0] || await browserContext.newPage();
  const resolvedInvites = new Set();

  // If a search query is provided, query Disboard first to discover server IDs
  if (searchQuery) {
    console.log(`[*] Searching Disboard for query: "${searchQuery}" (limit: ${searchLimit})...`);
    const searchUrl = `https://disboard.org/search?keyword=${encodeURIComponent(searchQuery)}`;
    await page.goto(searchUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await waitForCloudflare(page);

    // Extract all server join links from search results
    const foundIds = await page.evaluate(() => {
      const ids = [];
      const links = document.querySelectorAll('a[href*="/server/join/"], a[href*="/server/"]');
      for (const a of links) {
        const href = a.getAttribute('href') || '';
        const match = href.match(/\/server\/(?:join\/)?(\d{17,20})/);
        if (match && !ids.includes(match[1])) {
          ids.push(match[1]);
        }
      }
      return ids;
    });

    console.log(`[+] Disboard search returned ${foundIds.length} candidate servers.`);
    for (const id of foundIds.slice(0, searchLimit)) {
      if (!targets.includes(id)) {
        targets.push(id);
      }
    }
  }

  console.log(`[*] Resolving ${targets.length} target server(s)...\n`);

  for (let i = 0; i < targets.length; i++) {
    const raw = targets[i].trim();
    const serverId = extractServerId(raw);
    if (!serverId) {
      console.warn(`[!] Skipping invalid Disboard target: ${raw}`);
      continue;
    }

    const joinUrl = `https://disboard.org/server/join/${serverId}`;
    console.log(`[${i + 1}/${targets.length}] Resolving server ${serverId} (${joinUrl})...`);

    let capturedInvite = null;

    const checkUrl = (url) => {
      if (url.includes('discord.gg/') || url.includes('discord.com/invite/')) {
        capturedInvite = url;
        return true;
      }
      return false;
    };

    const onRequest = (req) => checkUrl(req.url());
    page.on('request', onRequest);

    try {
      await page.goto(joinUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
      checkUrl(page.url());

      // Wait if Cloudflare Turnstile interstitial appears
      await waitForCloudflare(page);
      checkUrl(page.url());

      if (!capturedInvite) {
        // Check for 18+ age verification button
        const confirm18Btn = page.locator('button:has-text("18+"), a:has-text("18+"), text="Yes, I am 18+", text="Yes, I\'m 18+"').first();
        if (await confirm18Btn.isVisible({ timeout: 2000 }).catch(() => false)) {
          console.log('    [+] Clicking 18+ age confirmation...');
          await confirm18Btn.click().catch(() => {});
        }

        // Check for Disboard join button
        const joinBtn = page.locator('.server-join, a:has-text("Join this server"), button:has-text("Join this server")').first();
        if (await joinBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
          console.log('    [+] Clicking "Join this server" button...');
          await joinBtn.click().catch(() => {});
        }

        // Poll for redirect for up to 10 seconds
        const startTime = Date.now();
        while (!capturedInvite && Date.now() - startTime < 10000) {
          if (checkUrl(page.url())) break;
          await page.waitForTimeout(500);
        }
      }

      if (capturedInvite) {
        const cleanInvite = cleanDiscordInvite(capturedInvite);
        console.log(`    [✓] Extracted: ${cleanInvite}`);
        if (!resolvedInvites.has(cleanInvite)) {
          resolvedInvites.add(cleanInvite);
          fs.appendFileSync(outputFile, cleanInvite + '\n');
        }
      } else {
        console.warn(`    [!] Could not extract invite for server ${serverId}.`);
      }
    } catch (err) {
      console.error(`    [-] Error resolving ${joinUrl}: ${err.message}`);
    } finally {
      page.off('request', onRequest);
    }

    await page.waitForTimeout(1000);
  }

  await browserContext.close();

  console.log(`\n======================================================================`);
  console.log(`[✓] Successfully resolved ${resolvedInvites.size} Discord invite(s) to: ${outputFile}`);
  console.log(`======================================================================`);
  console.log(`You can now run your investigation with zero manual steps:`);
  console.log(`  ./bin/discord-osint search --invites-file ${outputFile} --username <TARGET>\n`);
}

async function waitForCloudflare(page) {
  for (let attempt = 0; attempt < 15; attempt++) {
    const title = await page.title().catch(() => '');
    const content = await page.content().catch(() => '');
    const isChallenge = title.includes('Just a moment...') ||
                        content.includes('cf-turnstile') ||
                        content.includes('Checking if the site connection is secure') ||
                        content.includes('Enable JavaScript and cookies to continue');
    if (!isChallenge) {
      return true;
    }
    if (attempt === 0) {
      console.log('    [!] Cloudflare Turnstile detected. If prompted, complete the verification in Chrome...');
    }
    await page.waitForTimeout(1000);
  }
  return false;
}

function extractServerId(input) {
  input = input.trim();
  if (/^\d{17,20}$/.test(input)) {
    return input;
  }
  const match = input.match(/disboard\.org\/server\/(?:join\/)?(\d{17,20})/);
  if (match) {
    return match[1];
  }
  return null;
}

function cleanDiscordInvite(url) {
  try {
    const parsed = new URL(url);
    return `${parsed.origin}${parsed.pathname}`;
  } catch {
    return url.split('?')[0];
  }
}

function printHelp() {
  console.log(`
Disboard -> discord.gg Automated Resolver

Usage:
  node scripts/resolve-disboard.js [options] [server_id_or_url...]

Options:
  -s, --search <query>  Search Disboard for a keyword or tag to automatically gather server IDs
  -l, --limit <n>       Maximum servers to resolve from search (default: 15)
  -f, --file <path>     Read Disboard server IDs or URLs from file
  -o, --output <path>   Write resolved discord.gg URLs to file (default: invites.txt)
  --headless            Run browser in headless mode (default: false)
  --profile <path>      Path to persistent Chrome profile directory
  -h, --help            Show this help message

Examples:
  node scripts/resolve-disboard.js 1164734637927571477
  node scripts/resolve-disboard.js --search "crypto" --limit 10 -o crypto_invites.txt
  node scripts/resolve-disboard.js -f targets.txt -o invites.txt
`);
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
