// 1. DOM references — grab elements once at top
const views = {
    create:  document.getElementById('view-create'),
    created: document.getElementById('view-created'),
    paste:   document.getElementById('view-paste')
}

const el = {
    pasteInput:   document.getElementById('paste-input'),
    expirySelect: document.getElementById('expiry-select'),
    createBtn:    document.getElementById('create-btn'),
    createError:  document.getElementById('create-error'),
    shareUrl:     document.getElementById('share-url'),
    ownerToken:   document.getElementById('owner-token'),
    newPasteBtn:  document.getElementById('new-paste-btn'),
    copyBtns:     document.querySelectorAll('.copy-btn'),
    burnNotice:   document.getElementById('burn-notice'),
    pasteContent: document.getElementById('paste-content'),
    showDeleteBtn: document.getElementById('show-delete-btn'),
    deleteForm:   document.getElementById('delete-form'),
    deleteTokenInput: document.getElementById('delete-token-input'),
    confirmDeleteBtn: document.getElementById('confirm-delete-btn'),
    deleteError:  document.getElementById('delete-error')
}

// 2. View helpers
function showView(name) {
    Object.values(views).forEach(v => v.classList.add('hidden'))
    views[name].classList.remove('hidden')
}

// 3. Router (runs on DOMContentLoaded)
// pathname === '/'       → showView('create')
// pathname === '/{id}'  → this is a paste URL
    // has hash (#key=…) → loadPaste(id, key)   ← function we'll write
    // no hash           → show an error (can't decrypt without the key)

document.addEventListener('DOMContentLoaded', () => {
    const path = window.location.pathname

    if (path === '/') {
        showView('create')
    } else {
        const id = path.slice(1)
        const params = new URLSearchParams(window.location.hash.slice(1))
        const keyRaw = params.get('key')
        
        if (keyRaw) {
            loadPaste(id, keyRaw)
        } else {
            showView('paste')
            el.pasteContent.textContent = 'Error: missing decryption key.'
        }
    }
})

function loadPaste(id, key) {
    console.log('Loading paste', id, 'with key', key)
    // 1. Fetch paste data from API (GET /api/paste/{id})
    // 2. Decode base64url ciphertext and IV
    // 3. Use Web Crypto API to decrypt ciphertext with key and IV
    // 4. Display decrypted content in pasteContent element
    // 5. If BurnAfterRead, show burnNotice and hide pasteContent until user clicks "Show Paste"
}

// 4. Crypto helpers
function bufToBase64(buf) {
    return btoa(String.fromCharCode(...new Uint8Array(buf)))
}

function base64ToBuf(b64) {
    return Uint8Array.from(atob(b64), c => c.charCodeAt(0))
}

async function generateKey() {
    return crypto.subtle.generateKey(
        { name: 'AES-GCM', length: 256 },
        true,
        ['encrypt', 'decrypt']
    )
}

async function encrypt(key, plaintext) {
    const iv = crypto.getRandomValues(new Uint8Array(12))
    const encoded = new TextEncoder().encode(plaintext)
    const ciphertext = await crypto.subtle.encrypt(
        { name: 'AES-GCM', iv },
        key,
        encoded
    )
    return {
        ciphertext: bufToBase64(ciphertext),
        iv: bufToBase64(iv)
    }
}

async function decrypt(key, ciphertext, iv) {
    const decodedCiphertext = base64ToBuf(ciphertext)
    const decodedIv = base64ToBuf(iv)
    const plaintext = await crypto.subtle.decrypt(
        { name: 'AES-GCM', iv: decodedIv },
        key,
        decodedCiphertext
    )
    return new TextDecoder().decode(plaintext)
}

// 5. API helpers
async function createPaste(ciphertext, iv, burnAfterRead, expiry) {
    const res = await fetch('/pastes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ciphertext, iv, burn_after_read: burnAfterRead, expiry })
    })
    if (!res.ok) throw new Error(await res.text())
    return res.json()
}

async function getPaste(id) {
    const res = await fetch(`/pastes/${id}`)
    if (!res.ok) throw new Error(await res.text())
    return res.json()
}

async function deletePaste(id, ownerToken) {
    const res = await fetch(`/pastes/${id}`, {
        method: 'DELETE',
        headers: { 'Authorization': `Bearer ${ownerToken}` }
    })
    if (!res.ok) throw new Error(await res.text())
}

// 6. Event handlers
el.createBtn.addEventListener('click', async () => {
    // implement create paste flow:
    // 1. Generate random key
    // 2. Encrypt pasteInput.value with key, get ciphertext and IV
    // 3. Call createPaste API helper with ciphertext, IV, burnAfterRead, expiry
    // 4. Show created view with share URL (including key in hash) and owner token
})

