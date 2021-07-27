package gcsemu

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/google/btree"
	"google.golang.org/api/storage/v1"
)

type memstore struct {
	mu      sync.RWMutex
	buckets map[string]*memBucket
}

var _ Store = (*memstore)(nil)

func NewMemStore() *memstore {
	return &memstore{buckets: map[string]*memBucket{}}
}

type memBucket struct {
	created time.Time

	// mutex required (despite lock map in gcsemu), because btree mutations are not structurally safe
	mu    sync.RWMutex
	files *btree.BTree
}

func (ms *memstore) getBucket(bucket string) *memBucket {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.buckets[bucket]
}

type memFile struct {
	meta storage.Object
	data []byte
}

func (mf *memFile) Less(than btree.Item) bool {
	// TODO(dragonsinth): is a simple lexical sort ok for Walk?
	return mf.meta.Name < than.(*memFile).meta.Name
}

var _ btree.Item = (*memFile)(nil)

func (ms *memstore) CreateBucket(bucket string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if ms.buckets[bucket] == nil {
		ms.buckets[bucket] = &memBucket{
			created: time.Now(),
			files:   btree.New(16),
		}
	}
	return nil
}

func (ms *memstore) GetBucketMeta(bucket string) (*storage.Bucket, error) {
	if b := ms.getBucket(bucket); b != nil {
		obj := bucketMeta(bucket)
		obj.Updated = b.created.UTC().Format(time.RFC3339Nano)
		return obj, nil
	}
	return nil, nil
}

func (ms *memstore) Get(bucket string, filename string) (*storage.Object, []byte, error) {
	f := ms.find(bucket, filename)
	if f != nil {
		return &f.meta, f.data, nil
	}
	return nil, nil, nil
}

func (ms *memstore) GetMeta(bucket string, filename string) (*storage.Object, error) {
	f := ms.find(bucket, filename)
	if f != nil {
		meta := f.meta
		initMeta(&meta, bucket, filename, uint64(len(f.data)))
		return &meta, nil
	}
	return nil, nil
}

func (ms *memstore) Add(bucket string, filename string, contents []byte, meta *storage.Object) error {
	_ = ms.CreateBucket(bucket)

	initMeta(meta, bucket, filename, uint64(len(contents)))
	scrubMeta(meta)
	meta.Metageneration = 1

	// Cannot be overridden by caller
	now := time.Now().UTC()
	meta.Updated = now.UTC().Format(time.RFC3339Nano)
	meta.Generation = now.UnixNano()
	if meta.TimeCreated == "" {
		meta.TimeCreated = meta.Updated
	}

	b := ms.getBucket(bucket)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files.ReplaceOrInsert(&memFile{
		meta: *meta,
		data: contents,
	})
	return nil
}

func (ms *memstore) UpdateMeta(bucket string, filename string, meta *storage.Object, metagen int64) error {
	f := ms.find(bucket, filename)
	if f == nil {
		return os.ErrNotExist
	}

	initMeta(meta, bucket, filename, 0)
	scrubMeta(meta)
	meta.Metageneration = metagen

	b := ms.getBucket(bucket)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files.ReplaceOrInsert(&memFile{
		meta: *meta,
		data: f.data,
	})
	return nil
}

func (ms *memstore) Copy(srcBucket string, srcFile string, dstBucket string, dstFile string) (*storage.Object, error) {
	src := ms.find(srcBucket, srcFile)
	if src == nil {
		return nil, nil
	}

	// Copy with metadata
	meta := src.meta
	meta.TimeCreated = "" // reset creation time on the dest file
	err := ms.Add(dstBucket, dstFile, src.data, &meta)
	if err != nil {
		return nil, err
	}

	// Reread the updated metadata and return it.
	return ms.GetMeta(dstBucket, dstFile)
}

func (ms *memstore) Delete(bucket string, filename string) error {
	if filename == "" {
		// Remove the bucket
		delete(ms.buckets, bucket)
	} else if b := ms.getBucket(bucket); b != nil {
		// Remove just the file
		b.mu.Lock()
		defer b.mu.Unlock()
		b.files.Delete(ms.key(filename))
	}

	return nil
}

func (ms *memstore) ReadMeta(bucket string, filename string, _ os.FileInfo) (*storage.Object, error) {
	return ms.GetMeta(bucket, filename)
}

func (ms *memstore) Walk(ctx context.Context, bucket string, cb func(ctx context.Context, filename string, fInfo os.FileInfo) error) error {
	if b := ms.getBucket(bucket); b != nil {
		var err error
		b.mu.RLock()
		defer b.mu.RUnlock()
		b.files.Ascend(func(i btree.Item) bool {
			mf := i.(*memFile)
			err = cb(ctx, mf.meta.Name, nil)
			return err == nil
		})
		return nil
	}
	return os.ErrNotExist
}

func (ms *memstore) key(filename string) btree.Item {
	return &memFile{
		meta: storage.Object{
			Name: filename,
		},
	}
}

func (ms *memstore) find(bucket string, filename string) *memFile {
	if b := ms.getBucket(bucket); b != nil {
		b.mu.Lock()
		defer b.mu.Unlock()
		f := b.files.Get(ms.key(filename))
		if f != nil {
			return f.(*memFile)
		}
	}
	return nil
}
