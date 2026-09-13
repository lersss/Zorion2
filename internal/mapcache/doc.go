// Package mapcache — карта из снапшота в памяти: миры вселенной грузятся один
// раз при старте, кластеризация /api/worlds/filter выполняется в Go без SQL.
// Snapshot immutable, замена через atomic.Pointer, хот-пат без мьютексов.
package mapcache