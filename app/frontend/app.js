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
document.addEventListener('DOMContentLoaded', async () => {
    const path = window.location.pathname

    if (path === '/') {
        showView('create')
    } else {
        const id = path.slice(1)
        const params = new URLSearchParams(window.location.hash.slice(1))
        const keyRaw = params.get('key')
        
        if (keyRaw) {
            await loadPaste(id, keyRaw)
        } else {
            showView('paste')
            el.pasteContent.textContent = 'Error: missing decryption key.'
        }
    }
})

async function loadPaste(id, keyRaw) {
    showView('paste')
    try {
        const key = await importKey(keyRaw)
        const data = await getPaste(id)
        const plaintext = await decrypt(key, data.ciphertext, data.iv)

        el.pasteContent.textContent = plaintext

        if (data.burn_after_read) {
            el.burnNotice.classList.remove('hidden')
            document.getElementById('delete-section').classList.add('hidden')
        }
    } catch (err) {
        el.pasteContent.textContent = 'Error: could not load or decrypt paste.'
        console.error(err)
    }
}

// 4. Crypto helpers
function bufToBase64(buf) {
    let binary = ''
    const bytes = new Uint8Array(buf)
    for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i])
    }
    return btoa(binary)
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

async function exportKey(key) {
    const raw = await crypto.subtle.exportKey('raw', key)
    return bufToBase64(raw)
}

async function importKey(b64) {
    const raw = base64ToBuf(b64)
    return crypto.subtle.importKey(
        'raw',
        raw,
        { name: 'AES-GCM' },
        false,          // not extractable once imported
        ['decrypt']
    )
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
    el.createError.classList.add('hidden')
    el.createError.textContent = ''

    try {
        const plaintext = el.pasteInput.value.trim()
        if (!plaintext) throw new Error('Paste content cannot be empty.')

        const expiry = el.expirySelect.value
        const burnAfterRead = expiry === 'burn'

        const key = await generateKey()
        const { ciphertext, iv } = await encrypt(key, plaintext)
        const { id, owner_token } = await createPaste(ciphertext, iv, burnAfterRead, expiry)
        const keyB64 = await exportKey(key)

        el.shareUrl.value = `${window.location.origin}/${id}#key=${keyB64}`
        el.ownerToken.value = owner_token
        showView('created') 
    } catch (err) {
        el.createError.textContent = err.message
        el.createError.classList.remove('hidden')
    }
})
