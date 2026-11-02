const { test } = require('node:test');
const assert = require('node:assert/strict');
const { BoardTitle } = require('../web/static/js/board-title.js');
const { DocumentSync } = require('../web/static/js/document-sync.js');
const board = (title = '', revision = 0) => ({ title, title_revision: revision, revision: 3, code_html: 'server', code_css: '' });

test('renaming uses its own CAS and never mutates a conflicted document or its draft', async () => {
    const drafts = new Map();
    const doc = new DocumentSync(board(), { storage: { setItem:(k,v)=>drafts.set(k,v), getItem:k=>drafts.get(k), removeItem:k=>drafts.delete(k) }, key:'draft' });
    doc.edit({code_html:'local', code_css:''}); doc.receive({...board(), revision:4,code_html:'remote'});
    const before = JSON.stringify(doc.state()), saved = drafts.get('draft');
    const title = new BoardTitle(board(), { write: async request => {
        assert.deepEqual(request, {title:'Урок 🎉',base_title_revision:0});
        return {board:board(request.title,1)};
    } });
    title.edit('  Урок 🎉  '); await title.save();
    assert.equal(title.title,'Урок 🎉'); assert.equal(title.dirty,false);
    assert.equal(JSON.stringify(doc.state()),before); assert.equal(drafts.get('draft'),saved);
});

test('concurrent rename retains input until explicit retry or cancel', async () => {
    const requests=[];
    const title = new BoardTitle(board('old'), {write:async request=> {
        requests.push(request);
        return requests.length===1 ? {conflict:true,board:board('other',1)} : {board:board(request.title,2)};
    }});
    title.edit('mine'); await title.save();
    assert.equal(title.input,'mine'); assert.equal(title.title,'other'); assert.match(title.error,/уже изменили/);
    await title.save(); assert.equal(requests[1].base_title_revision,1); assert.equal(title.title,'mine');
    title.edit('pending'); title.receive(board('newer',4)); title.receive(board('stale',3));
    assert.equal(title.input,'pending'); assert.equal(title.title,'newer'); title.reset(); assert.equal(title.input,'newer');
});

test('lost response keeps input; retry can recognize the already committed title', async () => {
    let attempt=0;
    const title = new BoardTitle(board(),{write:async()=>{
        if (++attempt===1) throw Error('offline');
        return {conflict:true,board:board('mine',1)};
    }});
    title.edit('mine'); await title.save(); assert.equal(title.input,'mine'); assert.match(title.error,/не подтверждено/);
    await title.save(); assert.equal(title.error,''); assert.equal(title.dirty,false);
});

test('Unicode limit counts code points, empty resets title and invalid input never writes', async () => {
    let writes=0;
    const title = new BoardTitle(board(),{write:async request=>({board:board(request.title,++writes)})});
    for(const value of ['界'.repeat(101),'a\nb','a\u0085b']) {title.edit(value);await title.save();assert.equal(writes,0);}
    title.edit('🎉'.repeat(100)); await title.save(); assert.equal(writes,1);
    title.edit('  '); await title.save(); assert.equal(title.title,'');assert.equal(writes,2);
});

test('busy saves cannot duplicate requests and late responses cannot roll back newer metadata', async () => {
    let resolve, writes=0;
    const title = new BoardTitle(board(),{write:()=>{writes++;return new Promise(r=>resolve=r);}});
    title.edit('mine');const pending=title.save();await title.save();assert.equal(writes,1);
    title.receive(board('newer',2));resolve({board:board('mine',1)});await pending;
    assert.equal(title.title,'newer');assert.equal(title.input,'mine');assert.match(title.error,/снова изменили/);
});
