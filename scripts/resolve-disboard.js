#!/usr/bin/env node

/**
 * Disboard -> discord.gg Automated Resolver
 *
 * Uses genuine Chrome via Playwright with persistent storage and .env cookies to bypass Cloudflare
 * TLS fingerprinting, search server names, handle NSFW/age gates, and capture clean discord.gg/xxxx invites.
 *
 * Usage:
 *   # 1. Resolve Disboard server link(s) or IDs:
 *   node scripts/resolve-disboard.js https://disboard.org/server/123456789012345678
 *
 *   # 2. Or pass a file with server names / URLs / IDs (one per line):
 *   node scripts/resolve-disboard.js -f servers.txt -o invites.txt
 */

const fs = require('fs');
const path = require('path');

async function main() {
  const args = process.argv.slice(2);
  let inputFile = null;
  let outputFile = 'invites.txt';
  let profileDir = path.join(__dirname, '..', '.chrome-disboard-profile');
  let headless = false;
  let generalSearchQuery = null;
  let searchLimit = 15;
  let rawTargets = [];

  // Parse CLI args
  for (let i = 0; i < args.length; i++) {
    if (args[i] === '-f' || args[i] === '--file' || args[i] === '--input' || args[i] === '--names-file') {
      inputFile = args[++i];
    } else if (args[i] === '-o' || args[i] === '--output') {
      outputFile = args[++i];
    } else if (args[i] === '--headless') {
      headless = true;
    } else if (args[i] === '--profile') {
      profileDir = args[++i];
    } else if (args[i] === '-s' || args[i] === '--search' || args[i] === '--tag') {
      generalSearchQuery = args[++i];
    } else if (args[i] === '-l' || args[i] === '--limit') {
      searchLimit = parseInt(args[++i], 10) || 15;
    } else if (args[i] === '--names') {
      i++;
      while (i < args.length && !args[i].startsWith('-')) {
        if (args[i].includes(',')) {
          rawTargets.push(...args[i].split(',').map(s => s.trim()).filter(Boolean));
        } else {
          rawTargets.push(args[i].trim());
        }
        i++;
      }
      i--;
    } else if (args[i] === '-h' || args[i] === '--help') {
      printHelp();
      process.exit(0);
    } else if (!args[i].startsWith('-')) {
      rawTargets.push(args[i]);
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
        rawTargets.push(line);
      }
    }
  }

  if (rawTargets.length === 0 && !generalSearchQuery) {
    console.error('[-] No server names, Disboard IDs, or search queries provided.');
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
      console.error('[-] Playwright is not installed. Run: npm install -D playwright');
      process.exit(1);
    }
  }

  // Read cookies from .env
  let cookiesToInject = [];
  const envPath = path.join(__dirname, '..', '.env');
  if (fs.existsSync(envPath)) {
    const envContent = fs.readFileSync(envPath, 'utf-8');
    const m = envContent.match(/DISBOARD_COOKIES="?([^"\n]+)"?/);
    if (m && m[1]) {
      cookiesToInject = m[1].split(';').map(c => {
        const parts = c.trim().split('=');
        return {
          name: parts[0].trim(),
          value: parts.slice(1).join('=').trim(),
          domain: '.disboard.org',
          path: '/'
        };
      }).filter(c => c.name && c.value);
    }
  }

  console.log(`======================================================================`);
  console.log(`[*] Disboard -> discord.gg Automated Browser Resolver`);
  console.log(`[*] Chrome Profile : ${profileDir}`);
  console.log(`[*] Output File    : ${outputFile}`);
  if (cookiesToInject.length > 0) {
    console.log(`[*] Session Cookies: ${cookiesToInject.length} cookies loaded from .env`);
  }
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

  if (cookiesToInject.length > 0) {
    await browserContext.addCookies(cookiesToInject);
  }

  const page = browserContext.pages()[0] || await browserContext.newPage();
  const resolvedInvites = new Set();

  console.log(`[*] Processing ${rawTargets.length} target(s)...\n`);

  for (let i = 0; i < rawTargets.length; i++) {
    const item = rawTargets[i].trim();
    if (!item) continue;

    const directId = extractServerId(item);
    let serverId = directId;
    let displayName = item;

    // If item is not a direct server ID or URL, first check Discord's vanity directory
    if (!serverId) {
      const vanity = await tryVanityOrSlug(item);
      if (vanity) {
        console.log(`    [✓] Discovered direct Discord vanity invite: https://discord.gg/${vanity.code} ("${vanity.name}", ${vanity.members || 0} members)`);
        const cleanInvite = `https://discord.gg/${vanity.code}`;
        if (!resolvedInvites.has(cleanInvite)) {
          resolvedInvites.add(cleanInvite);
          fs.appendFileSync(outputFile, `# ${vanity.name} (${vanity.id || 'vanity'})\n${cleanInvite}\n`);
        }
        continue;
      }

      console.log(`[${i + 1}/${rawTargets.length}] Searching Disboard for server: "${item}"...`);
      const searchUrl = `https://disboard.org/search?keyword=${encodeURIComponent(item)}&nsfw=1`;
      try {
        await page.goto(searchUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
        await waitForCloudflare(page);

        // Find best matching server result
        const match = await page.evaluate((targetName) => {
          const lowerTarget = targetName.toLowerCase();
          const cards = document.querySelectorAll('.server-card, .listing, .server-item, .column');
          let best = null;

          for (const card of cards) {
            const joinLink = card.querySelector('a[href*="/server/join/"], a[href*="/server/"]');
            if (!joinLink) continue;
            const href = joinLink.getAttribute('href') || '';
            const m = href.match(/\/server\/(?:join\/)?(\d{17,20})/);
            if (!m) continue;

            const titleElem = card.querySelector('.server-name, .server-title, .listing-title, h2, h3, a');
            const title = titleElem ? titleElem.innerText.trim() : '';

            if (title.toLowerCase() === lowerTarget) {
              return { id: m[1], name: title, exact: true };
            }
            if (!best) {
              best = { id: m[1], name: title, exact: false };
            }
          }

          if (!best) {
            const allJoinLinks = document.querySelectorAll('a[href*="/server/join/"]');
            for (const a of allJoinLinks) {
              const href = a.getAttribute('href') || '';
              const m = href.match(/\/server\/join\/(\d{17,20})/);
              if (m) {
                return { id: m[1], name: a.innerText.trim() || targetName, exact: false };
              }
            }
          }

          return best;
        }, item);

        if (match && match.id) {
          serverId = match.id;
          displayName = match.name || item;
          console.log(`    [+] Found server: "${displayName}" (ID: ${serverId})`);
        } else {
          console.warn(`    [!] No Disboard search results for: "${item}". If you have its Disboard link, pass the URL directly.`);
          continue;
        }
      } catch (err) {
        console.error(`    [-] Error searching for "${item}": ${err.message}`);
        continue;
      }
    }

    // Resolve the invite link for serverId
    const joinUrl = `https://disboard.org/server/join/${serverId}`;
    console.log(`    [*] Joining server flow: ${joinUrl}`);

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
      await waitForCloudflare(page);
      checkUrl(page.url());

      if (!capturedInvite) {
        // Check for 18+ confirmation
        const confirm18Btn = page.locator('button:has-text("18+"), a:has-text("18+"), text="Yes, I am 18+", text="Yes, I\'m 18+"').first();
        if (await confirm18Btn.isVisible({ timeout: 2000 }).catch(() => false)) {
          console.log('    [+] Clicking 18+ age verification...');
          await confirm18Btn.click().catch(() => {});
        }

        // Check for join button
        const joinBtn = page.locator('.server-join, a:has-text("Join this server"), button:has-text("Join this server")').first();
        if (await joinBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
          console.log('    [+] Clicking "Join this server"...');
          await joinBtn.click().catch(() => {});
        }

        // Poll for redirect
        const startTime = Date.now();
        while (!capturedInvite && Date.now() - startTime < 10000) {
          if (checkUrl(page.url())) break;
          await page.waitForTimeout(500);
        }
      }

      if (capturedInvite) {
        const cleanInvite = cleanDiscordInvite(capturedInvite);
        console.log(`    [✓] Captured invite for "${displayName}": ${cleanInvite}\n`);
        if (!resolvedInvites.has(cleanInvite)) {
          resolvedInvites.add(cleanInvite);
          fs.appendFileSync(outputFile, `# ${displayName} (${serverId})\n${cleanInvite}\n`);
        }
      } else {
        console.warn(`    [!] Could not capture invite for "${displayName}" (ID: ${serverId}).\n`);
      }
    } catch (err) {
      console.error(`    [-] Error resolving ${joinUrl}: ${err.message}\n`);
    } finally {
      page.off('request', onRequest);
    }

    await page.waitForTimeout(1000);
  }

  await browserContext.close();

  console.log(`======================================================================`);
  console.log(`[✓] Successfully resolved ${resolvedInvites.size} Discord invite(s) to: ${outputFile}`);
  console.log(`======================================================================`);
  console.log(`Now run your automated Discord OSINT scan:`);
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
      console.log('    [!] Cloudflare challenge detected. If prompted, complete the verification in Chrome...');
    }
    await page.waitForTimeout(1000);
  }
  return false;
}

async function tryVanityOrSlug(name) {
  const clean = name.toLowerCase().replace(/[^a-z0-9]/g, '');
  const kebab = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  const candidates = [clean, kebab];
  if (name.toLowerCase().startsWith('the ')) {
    const withoutThe = name.slice(4).trim().toLowerCase().replace(/[^a-z0-9]/g, '');
    candidates.push(withoutThe);
  }
  // Also handle known abbreviations/common patterns
  if (name.toLowerCase().includes('&')) {
    candidates.push(name.toLowerCase().replace('&', 'n').replace(/[^a-z0-9]/g, ''));
    candidates.push(name.toLowerCase().replace(/the /g, '').replace('&', 'n').replace(/[^a-z0-9]/g, ''));
  }

  for (const slug of Array.from(new Set(candidates))) {
    if (!slug) continue;
    try {
      const res = await fetch(`https://discord.com/api/v9/invites/${slug}?with_counts=true`);
      if (res.status === 200) {
        const data = await res.json();
        return {
          code: slug,
          name: data.guild?.name || name,
          id: data.guild?.id,
          members: data.approximate_member_count
        };
      }
    } catch {}
  }
  return null;
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
  # Resolve Disboard server link(s):
  node scripts/resolve-disboard.js https://disboard.org/server/123456789012345678

  # Read server links or names from a file:
  node scripts/resolve-disboard.js -f servers.txt -o invites.txt
`);
}

main().catch(err => {
  console.error('Fatal error:', err);
  process.exit(1);
});
