package session

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestSession crée une session de test avec des valeurs par défaut.
func newTestSession(sub, username, host string, port int) *Session {
	return &Session{
		Sub:          sub,
		Username:     username,
		ResourceType: "http",
		ResourceHost: host,
		ResourcePort: port,
		SourceIP:     "192.168.1.100",
		DecisionID:   "dec-test-001",
	}
}

// TestSessionRegistration vérifie qu'on peut enregistrer une session et obtenir un ID unique.
func TestSessionRegistration(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	sess := newTestSession("user|alice", "alice", "lan-app", 80)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	sessionID, err := mgr.Register(sess, cancel)
	if err != nil {
		t.Fatalf("Register() erreur inattendue : %v", err)
	}
	if sessionID == "" {
		t.Error("Register() doit retourner un ID non vide")
	}
	// L'ID doit respecter le format UUID (8-4-4-4-12)
	if len(sessionID) != 36 {
		t.Errorf("Register() ID = %q, longueur attendue 36 (format UUID), obtenu %d", sessionID, len(sessionID))
	}

	// Vérifier l'ID est enregistré dans les compteurs
	if count := mgr.ActiveCount(); count != 1 {
		t.Errorf("ActiveCount() = %d, attendu 1", count)
	}
	if count := mgr.ActiveCountForSubject("user|alice"); count != 1 {
		t.Errorf("ActiveCountForSubject() = %d, attendu 1", count)
	}

	// Vérifier que le cancel n'a pas encore été appelé
	if cancelCalled {
		t.Error("cancel() ne doit pas être appelé lors du Register")
	}
}

// TestSessionLimits vérifie que la limite de maxConnsPerSubject (10) est respectée.
func TestSessionLimits(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	sub := "user|bob"
	for i := 0; i < maxConnsPerSubject; i++ {
		sess := newTestSession(sub, "bob", "lan-app", 80+i)
		_, err := mgr.Register(sess, func() {})
		if err != nil {
			t.Fatalf("Register() tentative %d sur %d : erreur inattendue %v", i+1, maxConnsPerSubject, err)
		}
	}

	// La 11e connexion doit échouer
	_, err := mgr.Register(newTestSession(sub, "bob", "lan-app", 9999), func() {})
	if err == nil {
		t.Errorf("Register() devrait échouer à la %de connexion (limite %d)", maxConnsPerSubject+1, maxConnsPerSubject)
	}
	if count := mgr.ActiveCountForSubject(sub); count != maxConnsPerSubject {
		t.Errorf("ActiveCountForSubject() = %d, attendu %d", count, maxConnsPerSubject)
	}

	// Un autre utilisateur ne doit pas être bloqué
	_, err = mgr.Register(newTestSession("user|carol", "carol", "lan-app", 80), func() {})
	if err != nil {
		t.Errorf("Register() autre utilisateur ne doit pas être bloqué : %v", err)
	}
}

// TestSessionExpiration vérifie que Unregister décrémente bien les compteurs après une "expiration".
func TestSessionExpiration(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	// Simuler une session "ancienne" (StartedAt dans le passé)
	sess := newTestSession("user|alice", "alice", "lan-app", 80)
	sess.StartedAt = time.Now().Add(-20 * time.Minute)

	sessionID, err := mgr.Register(sess, func() {})
	if err != nil {
		t.Fatalf("Register() : %v", err)
	}

	if mgr.ActiveCount() != 1 {
		t.Errorf("ActiveCount() = %d, attendu 1 avant expiration", mgr.ActiveCount())
	}

	// Simuler la fin de la session (le goroutine proxy appelle Unregister)
	mgr.Unregister(sessionID)

	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, attendu 0 après Unregister", mgr.ActiveCount())
	}
}

// TestSessionCleanup vérifie que Register + Unregister libèrent correctement les ressources.
func TestSessionCleanup(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	sub := "user|cleanup"
	var ids []string
	for i := 0; i < 5; i++ {
		id, err := mgr.Register(newTestSession(sub, "cleanup", "lan-app", 80+i), func() {})
		if err != nil {
			t.Fatalf("Register() session %d : %v", i, err)
		}
		ids = append(ids, id)
	}

	if mgr.ActiveCount() != 5 {
		t.Errorf("ActiveCount() = %d, attendu 5", mgr.ActiveCount())
	}

	// Unregister en ordre inversé
	for _, id := range ids {
		mgr.Unregister(id)
	}

	if mgr.ActiveCount() != 0 {
		t.Errorf("ActiveCount() = %d, attendu 0 après cleanup", mgr.ActiveCount())
	}

	// Unregister d'un ID inexistant ne doit pas paniquer
	mgr.Unregister("id-qui-nexiste-pas")
}

// TestSessionMetrics vérifie ActiveCount et ActiveCountForSubject avec plusieurs sujets.
func TestSessionMetrics(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	// alice : 3 sessions
	for i := 0; i < 3; i++ {
		_, err := mgr.Register(newTestSession("user|alice", "alice", "lan-app", 80+i), func() {})
		if err != nil {
			t.Fatalf("Register alice %d : %v", i, err)
		}
	}
	// bob : 2 sessions
	for i := 0; i < 2; i++ {
		_, err := mgr.Register(newTestSession("user|bob", "bob", "backend", 8080+i), func() {})
		if err != nil {
			t.Fatalf("Register bob %d : %v", i, err)
		}
	}

	if total := mgr.ActiveCount(); total != 5 {
		t.Errorf("ActiveCount() = %d, attendu 5", total)
	}
	if n := mgr.ActiveCountForSubject("user|alice"); n != 3 {
		t.Errorf("ActiveCountForSubject(alice) = %d, attendu 3", n)
	}
	if n := mgr.ActiveCountForSubject("user|bob"); n != 2 {
		t.Errorf("ActiveCountForSubject(bob) = %d, attendu 2", n)
	}
	if n := mgr.ActiveCountForSubject("user|inexistant"); n != 0 {
		t.Errorf("ActiveCountForSubject(inexistant) = %d, attendu 0", n)
	}
}

// TestKillSessionCallsCancel vérifie que KillSession appelle bien la cancel func et retourne true.
func TestKillSessionCallsCancel(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	// Contexte lié à la session
	ctx, cancel := context.WithCancel(context.Background())
	sessionID, err := mgr.Register(newTestSession("user|alice", "alice", "lan-app", 80), cancel)
	if err != nil {
		t.Fatalf("Register() : %v", err)
	}

	// Vérifier que le contexte est actif
	select {
	case <-ctx.Done():
		t.Fatal("contexte annulé avant KillSession")
	default:
	}

	// Kill de la session
	killed := mgr.KillSession(sessionID)
	if !killed {
		t.Error("KillSession() devrait retourner true pour un ID existant")
	}

	// Le contexte doit être annulé
	select {
	case <-ctx.Done():
		// attendu
	case <-time.After(100 * time.Millisecond):
		t.Error("cancel() n'a pas été appelé après KillSession")
	}

	// KillSession sur un ID inexistant doit retourner false
	if mgr.KillSession("inexistant-id") {
		t.Error("KillSession() sur ID inexistant doit retourner false")
	}
}

// TestKillRevokedTerminatesSessions vérifie que KillRevoked annule les sessions dont le serial est révoqué.
func TestKillRevokedTerminatesSessions(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	cancelCount := int64(0)
	makeCancel := func() context.CancelFunc {
		return func() { atomic.AddInt64(&cancelCount, 1) }
	}

	// Enregistrer 3 sessions avec serial révoqué : "deadbeef"
	for i := 0; i < 3; i++ {
		sess := newTestSession("user|alice", "alice", "lan-app", 80+i)
		sess.DeviceSerial = "deadbeef"
		_, err := mgr.Register(sess, makeCancel())
		if err != nil {
			t.Fatalf("Register() session avec serial révoqué : %v", err)
		}
	}
	// Enregistrer 2 sessions avec un serial différent (ne doit pas être killé)
	for i := 0; i < 2; i++ {
		sess := newTestSession("user|bob", "bob", "lan-app", 8080+i)
		sess.DeviceSerial = "cafecafe"
		_, err := mgr.Register(sess, makeCancel())
		if err != nil {
			t.Fatalf("Register() session non révoquée : %v", err)
		}
	}

	mgr.KillRevoked([]string{"deadbeef"})

	// Seulement les 3 sessions avec "deadbeef" doivent être annulées
	time.Sleep(10 * time.Millisecond) // laisser les cancel s'exécuter
	if got := atomic.LoadInt64(&cancelCount); got != 3 {
		t.Errorf("KillRevoked() a annulé %d sessions, attendu 3", got)
	}

	// KillRevoked avec slice vide ne doit pas paniquer
	mgr.KillRevoked([]string{})
	mgr.KillRevoked(nil)
}

// TestSessionConcurrentAccess vérifie la sécurité concurrente du Manager (pas de race condition).
func TestSessionConcurrentAccess(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mgr := NewManager(log)

	const numGoroutines = 8
	const sessionsPerGoroutine = 1 // 8 < maxConnsPerSubject (10)

	var wg sync.WaitGroup
	var registered int64

	// Enregistrer des sessions depuis plusieurs goroutines simultanément
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			sess := newTestSession("user|concurrent", "concurrent", "lan-app", 80+id)
			_, err := mgr.Register(sess, func() {})
			if err == nil {
				atomic.AddInt64(&registered, 1)
			}
		}(i)
	}
	wg.Wait()

	// Toutes les sessions (≤ maxConnsPerSubject) doivent être enregistrées
	if got := atomic.LoadInt64(&registered); got != numGoroutines*sessionsPerGoroutine {
		t.Errorf("sessions enregistrées = %d, attendu %d", got, numGoroutines*sessionsPerGoroutine)
	}

	// Lire le count concurrent pendant des Unregister simultanés
	var ids []string
	for id := range mgr.sessions {
		ids = append(ids, id)
	}

	wg.Add(len(ids))
	for _, id := range ids {
		go func(sid string) {
			defer wg.Done()
			mgr.Unregister(sid)
		}(id)
	}
	wg.Wait()

	if count := mgr.ActiveCount(); count != 0 {
		t.Errorf("ActiveCount() = %d après cleanup concurrent, attendu 0", count)
	}
}
